package store

import (
	"errors"
	"testing"

	"go.mongodb.org/mongo-driver/v2/mongo"
)

func TestIsIndexNotFound(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"index not found", mongo.CommandError{Code: 27, Name: "IndexNotFound"}, true},
		{"namespace not found", mongo.CommandError{Code: 26, Name: "NamespaceNotFound"}, true},
		{"other command error", mongo.CommandError{Code: 13, Name: "Unauthorized"}, false},
		{"plain error", errors.New("boom"), false},
	}
	for _, c := range cases {
		if got := isIndexNotFound(c.err); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}
