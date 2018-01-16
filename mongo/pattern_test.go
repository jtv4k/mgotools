package mongo

import (
	"reflect"
	"testing"
)

func TestPattern_NewPattern(t *testing.T) {
	s := []Object{
		{"a": 5},
		{"a": 5, "b": "y"},
		{"a": Object{"$in": "y"}},
		{"a": Object{"$gt": 5}},
		{"a": Object{"$exists": true}},
		{"$or": Array{Object{"a": 5}, Object{"b": 5}}},
		{"$and": Array{Object{"$or": Array{Object{"a": 5}, Object{"b": 5}}}, Array{Object{"$or": Array{Object{"c": 5}, Object{"d": 5}}}}}},
		{"_id": ObjectId{}},
		{"a": Object{"$in": Array{5, 5, 5}}},
		{"a": Object{"$elemMatch": Object{"b": 5, "c": Object{"$gte": 5}}}},
		{"a": Object{"$geoWithin": Object{"$center": Array{Array{5, 5}, 5}}}},
		{"a": Object{"$geoWithin": Object{"$geometry": Object{"a": "y", "b": Array{5, 5}}}}},
	}
	d := []Object{
		{"a": V{}},
		{"a": V{}, "b": V{}},
		{"a": Object{"$in": V{}}},
		{"a": Object{"$gt": V{}}},
		{"a": Object{"$exists": V{}}},
		{"$or": Array{Object{"a": V{}}, Object{"b": V{}}}},
		{"$and": Array{Object{"$or": Array{Object{"a": V{}}, Object{"b": V{}}}}, Array{Object{"$or": Array{Object{"c": V{}}, Object{"d": V{}}}}}}},
		{"_id": V{}},
		{"a": Object{"$in": V{}}},
		{"a": Object{"$elemMatch": Object{"b": V{}, "c": Object{"$gte": V{}}}}},
		{"a": Object{"$geoWithin": Object{"$center": Array{V{}, V{}}}}},
		{"a": Object{"$geoWithin": Object{"$geometry": Object{"a": V{}, "b": V{}}}}},
	}
	if len(s) != len(d) {
		t.Fatalf("mismatch between array sizes, %d and %d", len(s), len(d))
	}

	for i := range s {
		if p := NewPattern(s[i]); !reflect.DeepEqual(p.pattern, d[i]) {
			t.Errorf("pattern mismatch at %d: %#v %#v", i, s[i], d[i])
		}
	}
}
func TestPattern_Equals(t *testing.T) {
	s := []Object{
		{},
		{"a": V{}},
		{"a": V{}, "b": V{}},
		{"a": Object{"b": V{}}},
		{"a": Array{}},
		{"a": Array{V{}, V{}}},
		{"a": Object{}},
		{"a": Array{Object{"a": V{}}}},
		{"a": V{}},
	}
	d := []Object{
		{"a": V{}},
		{"b": V{}},
		{"b": V{}, "a": V{}, "c": V{}},
		{"a": Object{"c": V{}}},
		{"a": Array{V{}}},
		{"a": Array{V{}}},
		{"a": Array{}},
		{"a": Array{Object{"b": V{}}}},
		{"a": Object{}},
	}

	if len(s) != len(d) {
		t.Fatalf("mismatch between array sizes, %d and %d", len(s), len(d))
	}

	for i := range s {
		p := Pattern{s[i], true}
		if !p.Equals(Pattern{s[i], true}) {
			t.Errorf("equality mismatch at %d: %#v", i, s[i])
		}
	}
	for i := range s {
		p := Pattern{s[i], true}
		r := Pattern{d[i], true}
		if p.Equals(r) {
			t.Errorf("equality match at %d:\n%#v\n%v", i, s[i], d[i])
		}
	}
}
func TestPattern_IsEmpty(t *testing.T) {
	p := Pattern{}
	if !p.IsEmpty() {
		t.Errorf("unexpected initialized variable")
	}
	r := NewPattern(Object{})
	if r.IsEmpty() {
		t.Errorf("unexpected uninitialzied value")
	}
}
func TestPattern_Pattern(t *testing.T) {
	p := Pattern{}
	if p.Pattern() != nil {
		t.Errorf("pattern should be empty")
	}
}

var patterns = []Pattern{
	NewPattern(Object{}),
	NewPattern(Object{"a": 1}),
	NewPattern(Object{"a": 1, "b": 1}),
	NewPattern(Object{"a": Object{"b": 1}}),
	NewPattern(Object{"a": Object{"b": Array{1, 1}}}),
	NewPattern(Object{"a": Object{"b": Object{"c": Object{"d": 1}}}}),
	NewPattern(Object{"a": Array{Object{"a": 1}, Object{"b": 1}, Object{"c": 1}}}),
}

func BenchmarkPattern_Equals(b *testing.B) {
	for i := 0; i < b.N; i += 1 {
		for _, s := range patterns {
			s.Equals(s)
		}
	}
}
func BenchmarkReflection_DeepCopy(b *testing.B) {
	for i := 0; i < b.N; i += 1 {
		for _, s := range patterns {
			reflect.DeepEqual(s, s)
		}
	}
}
