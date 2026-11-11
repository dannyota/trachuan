package embedding

import (
	"fmt"
	"reflect"
	"strings"
)

func recipeDifferences(expected, running Recipe) string {
	e, r := reflect.ValueOf(expected), reflect.ValueOf(running)
	fields := e.Type()
	var differences []string
	for i := 0; i < e.NumField(); i++ {
		if e.Field(i).Interface() == r.Field(i).Interface() {
			continue
		}
		name := fields.Field(i).Tag.Get("json")
		format := "%v"
		if e.Field(i).Kind() == reflect.String {
			format = "%q"
		}
		differences = append(differences, fmt.Sprintf("%s: expected "+format+", running "+format, name, e.Field(i).Interface(), r.Field(i).Interface()))
	}
	return strings.Join(differences, "; ")
}
