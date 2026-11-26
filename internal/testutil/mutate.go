// Package testutil holds helpers shared by tests across packages.
package testutil

import (
	"reflect"
	"time"
)

// Mutate sets v to a value different from its current one and reports whether
// it could. Structs change their first settable field; slices gain an element;
// maps are replaced with a one-entry map.
func Mutate(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.String:
		v.SetString(v.String() + "x")
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(v.Int() + 7)
	case reflect.Float32, reflect.Float64:
		v.SetFloat(v.Float() + 1.5)
	case reflect.Bool:
		v.SetBool(!v.Bool())
	case reflect.Pointer:
		n := reflect.New(v.Type().Elem())
		if !v.IsNil() {
			n.Elem().Set(v.Elem())
		}
		if !Mutate(n.Elem()) {
			return false
		}
		v.Set(n)
	case reflect.Slice:
		e := reflect.New(v.Type().Elem()).Elem()
		Mutate(e)
		v.Set(reflect.Append(v, e))
	case reflect.Map:
		m := reflect.MakeMap(v.Type())
		e := reflect.New(v.Type().Elem()).Elem()
		if !Mutate(e) {
			return false
		}
		m.SetMapIndex(reflect.ValueOf("1").Convert(v.Type().Key()), e)
		v.Set(m)
	case reflect.Struct:
		if v.Type() == reflect.TypeOf(time.Time{}) {
			v.Set(reflect.ValueOf(time.Date(2026, 11, 26, 19, 0, 0, 0, time.UTC)))
			return true
		}
		for i := 0; i < v.NumField(); i++ {
			if v.Field(i).CanSet() && Mutate(v.Field(i)) {
				return true
			}
		}
		return false
	default:
		return false
	}
	return true
}
