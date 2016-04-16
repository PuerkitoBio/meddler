package meddler

import (
	"reflect"
	"testing"
	"time"
)

type Metadata struct {
	ID   uint64 `meddler:"id,pk"`
	Name string `meddler:"name"`
	*SubMeta
}

type SubMeta struct {
	Height *int `meddler:"height"`
}

type EmbedPerson struct {
	Metadata

	private   int
	Email     string
	Ephemeral int       `meddler:"-"`
	Age       int       `meddler:",zeroisnull"`
	Closed    time.Time `meddler:"closed,utctimez"`
}

func TestGetFieldsEmbed(t *testing.T) {
	data, err := getFields(reflect.TypeOf((*EmbedPerson)(nil)))
	if err != nil {
		t.Errorf("Error in getFields: %v", err)
		return
	}

	// see if everything checks out
	if len(data.fields) != 6 || len(data.columns) != 6 {
		t.Fatalf("Found %d/%d fields, expected 6", len(data.fields), len(data.columns))
	}
	structFieldEqual(t, data.fields[data.columns[0]], &structField{"id", []int{0}, false, registry["identity"]})
	structFieldEqual(t, data.fields[data.columns[0]], &structField{"Email", []int{2}, false, registry["identity"]})
	structFieldEqual(t, data.fields[data.columns[0]], &structField{"Email", []int{2}, false, registry["identity"]})
	structFieldEqual(t, data.fields[data.columns[0]], &structField{"Email", []int{2}, false, registry["identity"]})
	structFieldEqual(t, data.fields[data.columns[1]], &structField{"Age", []int{4}, false, registry["zeroisnull"]})
	structFieldEqual(t, data.fields[data.columns[2]], &structField{"closed", []int{5}, false, registry["utctimez"]})
}
