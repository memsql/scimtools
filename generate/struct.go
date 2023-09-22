package generate

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/memsql/scimtools/schema"
)

type StructGenerator struct {
	w *genWriter
	s schema.ReferenceSchema
	e []schema.ReferenceSchema

	ptr         bool
	addTags     func(a *schema.Attribute) map[string]string
	customTypes map[string]CustomType
}

type CustomType struct {
	PkgPrefix string // package id
	AttrName  string // name of the attribute
	TypeName  string // name of the custom type
}

func NewStructGenerator(s schema.ReferenceSchema, extensions ...schema.ReferenceSchema) (StructGenerator, error) {
	if s.Name == "" {
		return StructGenerator{}, errors.New("schema does not have a name")
	}

	for _, extension := range extensions {
		if extension.ID == "" || extension.Name == "" {
			return StructGenerator{}, errors.New("extension does not have a name/id")
		}
		sort.SliceStable(extension.Attributes, func(i, j int) bool {
			return strings.ToLower(extension.Attributes[i].Name) < strings.ToLower(extension.Attributes[j].Name)
		})
	}

	// check if id an externalId is present, add if not
	var id, eid bool
	for _, a := range s.Attributes {
		if strings.ToLower(a.Name) == "id" {
			id = true
		}
		if strings.ToLower(a.Name) == "externalid" {
			eid = true
		}
	}
	if !id {
		s.Attributes = append(s.Attributes, schema.IDAttribute)
	}
	if !eid {
		s.Attributes = append(s.Attributes, schema.ExternalIDAttribute)
	}

	sort.SliceStable(s.Attributes, func(i, j int) bool {
		return strings.ToLower(s.Attributes[i].Name) < strings.ToLower(s.Attributes[j].Name)
	})

	return StructGenerator{
		w:           newGenWriter(&bytes.Buffer{}),
		s:           s,
		e:           extensions,
		customTypes: map[string]CustomType{},
	}, nil
}

// UsePtr indicates whether the generator will use pointers if the attribute is not required.
func (g *StructGenerator) UsePtr(t bool) *StructGenerator {
	g.ptr = t
	return g
}

// AddTags enables setting fields tags when the attribute is has certain attribute fields such as required.
func (g *StructGenerator) AddTags(f func(a *schema.Attribute) (tags map[string]string)) *StructGenerator {
	g.addTags = f
	return g
}

// CustomTypes allows defining custom types for an attribute.
func (g *StructGenerator) CustomTypes(types []CustomType) *StructGenerator {
	if len(types) == 0 {
		return g
	}
	g.customTypes = make(map[string]CustomType)
	for _, t := range types {
		g.customTypes[t.AttrName] = t
	}
	return g
}

// Generate creates a buffer with a go representation of the resource described in the given schema.
func (g *StructGenerator) Generate() *bytes.Buffer {
	g.generateStruct(g.s.Name, g.s.Description, g.s.Attributes, true)
	for _, e := range g.e {
		g.w.n()
		g.generateStruct(e.Name+"Extension", e.Description, e.Attributes, false)
	}
	return g.w.writer.(*bytes.Buffer)
}

func (g *StructGenerator) generateStruct(name, desc string, attrs []*schema.Attribute, core bool) {
	w := g.w

	name = keepAlpha(name) // remove all non alpha characters

	if desc != "" {
		w.ln(comment(wrap(desc, 117))) // 120 - "// "
	}

	if len(attrs) == 0 {
		w.lnf("type %s struct {}", name)
		return
	}

	w.lnf("type %s struct {", name)
	g.generateStructFields(name, attrs, core)
	w.ln("}")

	for _, attr := range attrs {
		_, custom := g.customTypes[attr.Name]
		if attr.Type == schema.ComplexType && !custom {
			typ := cap(attr.Name)
			if attr.MultiValued {
				typ = singular(typ)
			}
			w.n()
			g.generateStruct(name+typ, attr.Description, attr.SubAttributes, false)
		}
	}
}

func (g *StructGenerator) generateStructFields(name string, attrs []*schema.Attribute, core bool) {
	w := g.w

	name = keepAlpha(name) // remove all non alpha characters

	// get longest name to indent fields.
	var indent int
	for _, attr := range attrs {
		if l := len(cap(attr.Name)); l > indent {
			indent = l
		}
	}

	for _, attr := range attrs {
		var typ string
		switch t := attr.Type; t {
		case "decimal":
			typ = "float64"
		case "integer":
			typ = "int"
		case "boolean":
			typ = "bool"
		case "complex":
			typ = cap(name + cap(attr.Name))
		default:
			typ = "string"
		}

		// field name
		name := cap(keepAlpha(attr.Name))
		w.in(4).w(name)
		w.sp(indent - len(name) + 1)

		if attr.MultiValued {
			w.w("[]")
			typ = singular(typ)
		} else if !attr.Required && g.ptr {
			w.w("*")
		}

		if t, custom := g.customTypes[attr.Name]; custom {
			if t.PkgPrefix != "" {
				typ = fmt.Sprintf("%s.%s", t.PkgPrefix, t.TypeName)
			} else {
				typ = t.TypeName
			}
		}

		if g.addTags != nil {
			tags := g.addTags(attr)
			w.w(typ)
			var tag string
			if tags != nil && len(tags) != 0 {
				for k, v := range tags {
					if v != "" {
						tag += fmt.Sprintf("%s:%q ", k, v)
					} else {
						tag += fmt.Sprintf("%s ", k)
					}
				}
				tag = strings.TrimSuffix(tag, " ")
				tag = fmt.Sprintf(" `%s`", tag)
			}
			w.ln(tag)
		} else {
			w.ln(typ)
		}
	}

	if core {
		// extensions
		if len(g.e) != 0 {
			w.n()
		}

		var indentE int
		for _, e := range g.e {
			if l := len(cap(keepAlpha(e.Name))); l > indentE {
				indentE = l
			}
		}
		for _, e := range g.e {
			name := cap(keepAlpha(e.Name))
			w.in(4).w(name)
			w.sp(indentE - len(name) + 1)
			typ := name + "Extension"
			w.w(typ)
			w.sp(indentE - len(typ) + 9)
			w.lnf(" `scim:%q`", e.ID)
		}
	}
}
