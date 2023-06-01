package marshal

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/muir/reflectutils"
)

var (
	ummarshalluuidType = reflect.TypeOf((*IDUnMarshaler)(nil)).Elem()
	unmarshalerType    = reflect.TypeOf((*Unmarshaler)(nil)).Elem()
	mapStringAnyType   = reflect.TypeOf(map[string]interface{}{})
	anySliceType       = reflect.TypeOf([]interface{}{})
)

func Unmarshal(data map[string]interface{}, value interface{}) error {
	v := reflect.ValueOf(value)
	if v.Kind() != reflect.Ptr || v.IsNil() {
		return errors.New("value is invalid")
	}

	t := v.Type()
	if t.Implements(unmarshalerType) {
		if v.Kind() == reflect.Ptr && v.IsNil() {
			return errors.New("ptr is nil")
		}
		m, ok := v.Interface().(Unmarshaler)
		if !ok {
			return errors.New("value does not implement marshaler")
		}
		return m.UnmarshalSCIM(data)
	}

	if t.Kind() == reflect.Ptr {
		v = v.Elem()
		t = v.Type()
	}

	var err error
	reflectutils.WalkStructElements(t, func(sf reflect.StructField) bool {
		tag := parseTags(sf)

		if sf.Anonymous {
			return true
		}

		if v := v.FieldByIndex(sf.Index); v.CanAddr() && v.CanSet() {
			name := lowerFirstRune(tag.name)

			if fV, ok := data[name]; ok {
				if fV == nil {
					return false
				}
				s := reflect.ValueOf(fV)
				switch s.Kind() {
				case reflect.Array, reflect.Slice:
					if t, ok := fV.([]map[string]interface{}); ok {
						field := reflect.MakeSlice(v.Type(), len(t), len(t))
						for i, v := range t {
							if reflect.ValueOf(v).Kind() == reflect.Map {
								t := toDefaultMap(v)
								typ := field.Index(i).Type()
								element := reflect.New(typ)
								initializeStruct(typ, element.Elem())
								if e := Unmarshal(t, element.Interface()); e != nil {
									err = e
								}
								field.Index(i).Set(element.Elem())
							} else {
								field.Index(i).Set(reflect.ValueOf(v))
							}
						}

						v.Set(field)

					} else {
						t := toDefaultSlice(fV)

						// if v.kind not match the value's kind, then skip
						if v.Kind() != reflect.Slice {
							break
						}
						field := reflect.MakeSlice(v.Type(), len(t), len(t))
						for i, v := range t {
							switch reflect.ValueOf(v).Kind() {
							case reflect.Map:
								t := toDefaultMap(v)
								typ := field.Index(i).Type()
								element := reflect.New(typ)
								initializeStruct(typ, element.Elem())
								if e := Unmarshal(t, element.Interface()); e != nil {
									err = e
								}
								field.Index(i).Set(element.Elem())
							default:
								field.Index(i).Set(reflect.ValueOf(v))
							}
						}
						v.Set(field)

					}

					return false
				case reflect.Map:
					t := toDefaultMap(fV)
					field := reflect.New(v.Type())
					initializeStruct(v.Type(), field.Elem())
					if e := Unmarshal(t, field.Interface()); e != nil {
						err = e
					}
					v.Set(field.Elem())
					return false
				}

				if s.Kind() != v.Kind() {
					err = fmt.Errorf(
						"types of %q do not match: got %s, want %s",
						name, s.Type(), v.Type(),
					)
				}

				if s.Type() != v.Type() {
					// special handle with uuid type
					if v.CanAddr() && v.Addr().Type().Implements(ummarshalluuidType) {
						m, ok := v.Addr().Interface().(IDUnMarshaler)
						if !ok {
							err = errors.New("value does not implement IDUnMarshaler")
						}
						err = m.UnmarshalSCIMUUID(fV)
					} else {
						v.Set(reflect.ValueOf(toType(fV, v.Type())))
					}

				} else {
					v.Set(s)
				}
			}
		}
		return false
	})
	if err != nil {
		return err
	}
	return nil
}

// Unmarshaler is the interface implemented by types that can unmarshal a SCIM description of themselves.
type Unmarshaler interface {
	UnmarshalSCIM(map[string]interface{}) error
}

type IDUnMarshaler interface {
	UnmarshalSCIMUUID(interface{}) error
}

func initializeStruct(t reflect.Type, v reflect.Value) {
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		ft := t.Field(i)
		switch ft.Type.Kind() {
		case reflect.Map:
			f.Set(reflect.MakeMap(ft.Type))
		case reflect.Slice:
			f.Set(reflect.MakeSlice(ft.Type, 0, 0))
		case reflect.Struct:
			initializeStruct(ft.Type, f)
		case reflect.Ptr:
			fv := reflect.New(ft.Type.Elem())
			initializeStruct(ft.Type.Elem(), fv.Elem())
			f.Set(fv)
		default:
		}
	}
}

func toDefaultMap(m interface{}) map[string]interface{} {
	if reflect.TypeOf(m) != mapStringAnyType {
		return toType(m, mapStringAnyType).(map[string]interface{})
	}
	return m.(map[string]interface{})
}

func toDefaultSlice(m interface{}) []interface{} {
	if reflect.TypeOf(m) != anySliceType {
		return toType(m, anySliceType).([]interface{})
	}
	return m.([]interface{})
}

func toType(i any, t reflect.Type) interface{} {
	// sqsq canconvert check
	return reflect.
		ValueOf(i).
		Convert(t).
		Interface()
}
