package cli

import (
	"reflect"
	"testing"
)

func TestParseCSVList(t *testing.T) {
	got := parseCSVList(" agent-a,agent-b,agent-a,*, ")
	want := []string{"agent-a", "agent-b", "*"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected parse result: got=%v want=%v", got, want)
	}
}
