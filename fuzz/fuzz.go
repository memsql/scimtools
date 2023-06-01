package fuzz

import (
	"encoding/base64"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/google/gofuzz"
	"github.com/memsql/scimtools/schema"
)

type Fuzzer struct {
	schema schema.ReferenceSchema

	fuzzer *fuzz.Fuzzer
	r      *rand.Rand

	emptyChance float64
	minElements int
	maxElements int
}

// New returns a new Fuzzer.
func New(schema schema.ReferenceSchema) *Fuzzer {
	r := rand.New(rand.NewSource(time.Now().Unix()))
	return &Fuzzer{
		schema:      schema,
		fuzzer:      fuzz.New().RandSource(r),
		r:           r,
		emptyChance: .2,
		minElements: 1,
		maxElements: 10,
	}
}

// EmptyChance sets the probability of creating an empty field map to the given chance.
// The chance should be between 0 (no empty fields) and 1 (all empty), inclusive.
func (f *Fuzzer) EmptyChance(p float64) *Fuzzer {
	if p < 0 || p > 1 {
		panic("p should be between 0 and 1, inclusive.")
	}
	f.emptyChance = p
	return f
}

// Fuzz recursively fills a Resource based on fields the ReferenceSchema of the Fuzzer.
func (f *Fuzzer) Fuzz() map[string]interface{} {
	var resource map[string]interface{}
	f.fuzzer.Funcs(f.newResourceFuzzer()).Fuzz(&resource)
	return resource
}

// NeverEmpty makes sure that all passed attribute names are never empty during fuzzing.
// Setting a complex attribute on never empty will also make their sub attributes never empty.
// i.e. "displayName", "name.givenName" or "emails.value"
func (f *Fuzzer) NeverEmpty(names ...string) *Fuzzer {
	for _, attribute := range f.schema.Attributes {
		for _, name := range names {
			neverEmpty(name, attribute)
		}
	}
	return f
}

func neverEmpty(name string, attribute *schema.Attribute) {
	n := strings.SplitN(name, ".", 2)
	if strings.EqualFold(n[0], attribute.Name) {
		if len(n) > 1 && attribute.Type == schema.ComplexType {
			for _, subAttribute := range attribute.SubAttributes {
				neverEmpty(n[1], subAttribute)
			}
		} else {
			attribute.Required = true
			if attribute.Type == schema.ComplexType {
				attribute.ForEachAttribute(func(attribute *schema.Attribute) {
					attribute.Required = true
				})
			}
		}
	}
}

func shouldFill(attribute *schema.Attribute) bool {
	return attribute.Required || attribute.Type == schema.ComplexType
}

// NumElements sets the minimum and maximum number of elements that will be added.
// If the elements are not required, it is possible to get less elements than the given parameter.
func (f *Fuzzer) NumElements(atLeast, atMost int) *Fuzzer {
	if atLeast > atMost {
		panic("atLeast must be <= atMost")
	}
	if atLeast < 0 {
		panic("atLeast must be >= 0")
	}
	f.minElements = atLeast
	f.maxElements = atMost
	return f
}

// RandSource causes the Fuzzer to get values from the given source of randomness.
func (f *Fuzzer) RandSource(s rand.Source) *Fuzzer {
	f.r = rand.New(s)
	f.fuzzer.RandSource(s)
	return f
}

func (f *Fuzzer) elementCount() int {
	if f.minElements == f.maxElements {
		return f.minElements
	}
	return f.minElements + f.r.Intn(f.maxElements-f.minElements+1)
}

func (f *Fuzzer) fuzzAttribute(resource map[string]interface{}, attribute *schema.Attribute, c fuzz.Continue) {
	if attribute.MultiValued {
		var elements []interface{}
		for i := 0; i < f.elementCount(); i++ {
			value := f.fuzzSingleAttribute(attribute, c)
			if value != nil {
				elements = append(elements, f.fuzzSingleAttribute(attribute, c))
			}
		}
		if len(elements) != 0 {
			resource[attribute.Name] = elements
		}
		return
	}

	if shouldFill(attribute) || f.shouldFill() {
		value := f.fuzzSingleAttribute(attribute, c)
		if value != nil {
			resource[attribute.Name] = f.fuzzSingleAttribute(attribute, c)
		}
	}
}

func (f *Fuzzer) fuzzSingleAttribute(attribute *schema.Attribute, c fuzz.Continue) interface{} {
	switch attribute.Type {
	case schema.StringType, schema.ReferenceType:
		var randString string
		if len(attribute.CanonicalValues) == 0 {
			randString = randAlphaString(c.Rand, 10)
		} else {
			randString = randStringFromSlice(c.Rand, attribute.CanonicalValues)
		}
		return randString
	case schema.BooleanType:
		randBool := c.RandBool()
		return randBool
	case schema.BinaryType:
		randBase64String := base64.StdEncoding.EncodeToString([]byte(randAlphaString(c.Rand, 10)))
		return randBase64String
	case schema.DecimalType:
		var randFloat64 float64
		c.Fuzz(&randFloat64)
		return randFloat64
	case schema.IntegerType:
		var randInt int
		c.Fuzz(&randInt)
		return randInt
	case schema.DateTimeType:
		randDateTimeString := randDateTime()
		return randDateTimeString
	case schema.ComplexType:
		complexResource := make(map[string]interface{})
		for _, subAttribute := range attribute.SubAttributes {
			f.fuzzAttribute(complexResource, subAttribute, c)
		}
		if len(complexResource) == 0 {
			return nil
		}
		return complexResource
	default:
		panic(fmt.Sprintf("unknown attribute type %s", attribute.Type))
	}
}

func (f *Fuzzer) newResourceFuzzer() func(resource *map[string]interface{}, c fuzz.Continue) {
	return func(r *map[string]interface{}, c fuzz.Continue) {
		resource := make(map[string]interface{})
		for _, attribute := range f.schema.Attributes {
			f.fuzzAttribute(resource, attribute, c)
		}
		*r = resource
	}
}

func (f *Fuzzer) shouldFill() bool {
	return f.r.Float64() > f.emptyChance
}
