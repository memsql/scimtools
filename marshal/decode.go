package marshal

import (
	"errors"
	"fmt"
	"reflect"

	// "github.com/Azure/azure-sdk-for-go/sdk/internal/uuid"
	"github.com/muir/reflectutils"
)

var (
	ummarshalluuidType = reflect.TypeOf((*IDUnMarshaler)(nil)).Elem()
	unmarshalerType    = reflect.TypeOf((*Unmarshaler)(nil)).Elem()
	mapType            = reflect.TypeOf(map[string]interface{}{})
	sliceType          = reflect.TypeOf([]interface{}{})
)

func Unmarshal(data map[string]interface{}, value interface{}) error {
	v := reflect.ValueOf(value)
	if v.Kind() != reflect.Ptr || v.IsNil() {
		// fmt.Printf("\nvalue is invalid value(%+v), v(%+v), v.Kind()(%+v)", value, v, v.Kind())
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

			//fmt.Printf("\ntag(%+v) data:(%+v)", tag, data[name])

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

					} else {
						t := toDefaultSlice(fV)

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
					// fmt.Printf("vtype is: (%+v), %T", v.Type().Name(), v.Type())
					// if v.Type().Name() == "uuid.SCIMResource" {
					// 	v.Set(reflect.ValueOf(fV).Interface().(uuid.UUID))
					// } else {
					// if v is ummarshalluuid
					// then unmarshaluuid(fv)
					// if v.CanAddr() {
					// 	fmt.Printf("before ummarshaluuid v(%+v), vcanaddr(%+v), vaddr(%+v), vaddrType(%+v)\n", v, v.CanAddr(), v.Addr(), v.Addr().Type())
					// }

					if v.CanAddr() && v.Addr().Type().Implements(ummarshalluuidType) {
						m, ok := v.Addr().Interface().(IDUnMarshaler)
						// m, ok := v.Interface().(IDUnMarshaler)
						if !ok {
							err = errors.New("value does not implement IDUnMarshaler")
							// return false
						}
						err = m.UnMarshalUUID(fV)
						// fmt.Printf("after ummarshaluuid m(%+v), v(%+v), fV(%+v)\n", m, v, fV)
						// if err != nil {
						// 	return false
						// }
					} else {
						v.Set(reflect.ValueOf(toType(fV, v.Type())))
					}

					// }

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

	// for i := 0; i < t.NumField(); i++ {
	// 	structFiled := t.Field(i)
	// 	tag := parseTags(structFiled)

	// 	// fix embedded
	// 	if structFiled.Anonymous {
	// 		if err := Unmarshal(data, v.Field(i).Addr().Interface()); err != nil {
	// 			return err
	// 		}
	// 		continue
	// 	}

	// 	if v := v.Field(i); v.CanAddr() && v.CanSet() {
	// 		name := lowerFirstRune(tag.name)

	// 		fmt.Printf("\ntag(%+v) data:(%+v)", tag, data[name])

	// 		if fV, ok := data[name]; ok {
	// 			if fV == nil {
	// 				continue
	// 			}
	// 			s := reflect.ValueOf(fV)
	// 			switch s.Kind() {
	// 			case reflect.Array, reflect.Slice:
	// 				t := toDefaultSlice(fV)
	// 				if v.Kind() != reflect.Slice {
	// 					break
	// 				}
	// 				field := reflect.MakeSlice(v.Type(), len(t), len(t))
	// 				for i, v := range t {
	// 					switch reflect.ValueOf(v).Kind() {
	// 					case reflect.Map:
	// 						t := toDefaultMap(v)
	// 						typ := field.Index(i).Type()
	// 						element := reflect.New(typ)
	// 						initializeStruct(typ, element.Elem())
	// 						if err := Unmarshal(t, element.Interface()); err != nil {
	// 							return err
	// 						}
	// 						field.Index(i).Set(element.Elem())
	// 					default:
	// 						field.Index(i).Set(reflect.ValueOf(v))
	// 					}
	// 				}
	// 				v.Set(field)
	// 				continue
	// 			case reflect.Map:
	// 				t := toDefaultMap(fV)
	// 				field := reflect.New(v.Type())
	// 				initializeStruct(v.Type(), field.Elem())
	// 				if err := Unmarshal(t, field.Interface()); err != nil {
	// 					return err
	// 				}
	// 				v.Set(field.Elem())
	// 				continue
	// 			}
	// 			if s.Kind() != v.Kind() {
	// 				return fmt.Errorf(
	// 					"types of %q do not match: got %s, want %s",
	// 					name, s.Type(), v.Type(),
	// 				)
	// 			}

	// 			if s.Type() != v.Type() {
	// 				v.Set(reflect.ValueOf(toType(fV, v.Type())))
	// 			} else {
	// 				v.Set(s)
	// 			}
	// 		}
	// 	}
	// }
	return nil
}

// Unmarshaler is the interface implemented by types that can unmarshal a SCIM description of themselves.
type Unmarshaler interface {
	UnmarshalSCIM(map[string]interface{}) error
}

type IDUnMarshaler interface {
	// uuid.scimresource
	UnMarshalUUID(interface{}) error
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
			// if !ft.Anonymous {
			initializeStruct(ft.Type, f)
			// }
		case reflect.Ptr:
			fv := reflect.New(ft.Type.Elem())
			initializeStruct(ft.Type.Elem(), fv.Elem())
			f.Set(fv)
		default:
		}
	}
}

func toDefaultMap(m interface{}) map[string]interface{} {
	if reflect.TypeOf(m) != mapType {
		return toType(m, mapType).(map[string]interface{})
	}
	return m.(map[string]interface{})
}

func toDefaultSlice(m interface{}) []interface{} {
	if reflect.TypeOf(m) != sliceType {
		return toType(m, sliceType).([]interface{})
	}
	return m.([]interface{})
}

func toType(i interface{}, t reflect.Type) interface{} {
	return reflect.
		ValueOf(i).
		Convert(t).
		Interface()
}
