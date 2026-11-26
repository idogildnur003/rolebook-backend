package activity

import (
	"reflect"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/elad/rolebook-backend/internal/model"
)

// maxValueLen is the longest text logged with its value. Longer text, and any
// text with a line break, is logged only as changed so prose never enters the
// history.
const maxValueLen = 40

// serverFields are identity and bookkeeping fields no user edit changes.
var serverFields = map[string]bool{
	"id": true, "_id": true, "campaignId": true, "createdAt": true, "updatedAt": true,
	"createdBy": true, "ownerPlayerId": true, "ownerUserId": true, "clone": true,
}

// DiffFields compares two values of the same struct type (or pointers to it)
// field by field, by JSON name, and returns one change per edited field.
// Numbers, booleans and short text carry before and after values; long text,
// lists, maps and images record only that they changed. Nested structs are
// compared field by field. Fields named in hide are never logged.
func DiffFields(before, after any, hide ...string) []model.ActivityChange {
	hidden := make(map[string]bool, len(hide))
	for _, h := range hide {
		hidden[h] = true
	}
	return diffValue(reflect.ValueOf(before), reflect.ValueOf(after), "", hidden)
}

func diffValue(b, a reflect.Value, label string, hidden map[string]bool) []model.ActivityChange {
	for b.Kind() == reflect.Pointer || b.Kind() == reflect.Interface {
		if b.IsNil() || a.IsNil() {
			if b.IsNil() && a.IsNil() {
				return nil
			}
			return []model.ActivityChange{changeFor(label, b, a)}
		}
		b, a = b.Elem(), a.Elem()
	}
	if b.Kind() == reflect.Struct && b.Type() != reflect.TypeOf(time.Time{}) {
		var out []model.ActivityChange
		t := b.Type()
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			name := jsonName(f)
			if !f.IsExported() || name == "" || serverFields[name] || hidden[name] {
				continue
			}
			out = append(out, diffValue(b.Field(i), a.Field(i), fieldLabel(name), hidden)...)
		}
		return out
	}
	if reflect.DeepEqual(b.Interface(), a.Interface()) || (isEmptyList(b) && isEmptyList(a)) {
		return nil
	}
	return []model.ActivityChange{changeFor(label, b, a)}
}

// changeFor builds the change for one field, keeping values only when they
// are short and plain.
func changeFor(label string, b, a reflect.Value) model.ActivityChange {
	before, okB := plainValue(b)
	after, okA := plainValue(a)
	if !okB || !okA || strings.HasSuffix(label, "image") {
		return model.ActivityChange{Field: label}
	}
	return model.ActivityChange{Field: label, Before: before, After: after}
}

// plainValue returns v as a loggable value. nil and "" become nil, shown as "—".
func plainValue(v reflect.Value) (any, bool) {
	for v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return nil, true
		}
		v = v.Elem()
	}
	switch v.Kind() {
	case reflect.Bool:
		return v.Bool(), true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return int(v.Int()), true
	case reflect.Float32, reflect.Float64:
		return v.Float(), true
	case reflect.String:
		s := v.String()
		if s == "" {
			return nil, true
		}
		if utf8.RuneCountInString(s) > maxValueLen || strings.ContainsAny(s, "\r\n") {
			return nil, false
		}
		return s, true
	case reflect.Struct:
		if t, ok := v.Interface().(time.Time); ok {
			if t.IsZero() {
				return nil, true
			}
			return t.UTC().Format("2006-01-02 15:04"), true
		}
	}
	return nil, false
}

func isEmptyList(v reflect.Value) bool {
	return (v.Kind() == reflect.Slice || v.Kind() == reflect.Map) && v.Len() == 0
}

func jsonName(f reflect.StructField) string {
	tag := f.Tag.Get("json")
	if tag == "-" {
		return ""
	}
	if name, _, _ := strings.Cut(tag, ","); name != "" {
		return name
	}
	return f.Name
}

// fieldLabel turns a JSON name into words: "linkedNpcIds" → "linked npcs",
// "parentLocationId" → "parent location", "thumbnailUri" → "thumbnail image".
func fieldLabel(name string) string {
	switch {
	case strings.HasSuffix(name, "Ids"):
		name = strings.TrimSuffix(name, "Ids") + "s"
	case strings.HasSuffix(name, "Id"):
		name = strings.TrimSuffix(name, "Id")
	case strings.HasSuffix(name, "ImageUri"):
		name = strings.TrimSuffix(name, "Uri")
	case strings.HasSuffix(name, "Uri"):
		name = strings.TrimSuffix(name, "Uri") + "Image"
	}
	var b strings.Builder
	for i, r := range name {
		if unicode.IsUpper(r) && i > 0 {
			b.WriteByte(' ')
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}
