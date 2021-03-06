package queryshape

// Borrowed from https://github.com/honeycombio/mongodbtools.
// But we get bson.M types instead of map[string]interface{} back from the BSON
// decoder, so type assertions in the original code of the form
// v.(map[string]interface{}) would fail. This is messy, but recursive type
// conversion would be even worse.

import (
	"sort"
	"strings"

	"gopkg.in/mgo.v2/bson"
)

// rough rules XXX(toshok) needs editing
// 1. if key is not an op:
//    1a. if value is a primitive, set value = 1
//    1b. if value is an aggregate, walk subtree flattening everything but ops and their values if necessary
// 2. if key is an op:
//    2a. if value is a primitive, set value = 1
//    2b. if value is a map, keep map + all keys, and process keys (starting at step 1)
//    2c. if value is a list, walk list.
//        2c1. if all values are primitive, set value = 1
//        2c2. if any values are maps/lists, keep map + all keys, and process keys (starting at step 1)

func GetQueryShape(q bson.M) string {
	if q_, ok := q["$query"].(bson.M); ok {
		return GetQueryShape(q_)
	}
	pruned := make(bson.M)
	for k, v := range q {
		if strings.HasPrefix(k, "$") || k == "filter" || k == "query" || k == "documents" {
			pruned[k] = flattenOp(v)
		} else {
			pruned[k] = flatten(v)
		}
	}
	// flatten pruned to a string, sorting keys alphabetically ($ coming before a/A)
	return serializeShape(pruned)
}

func isAggregate(v interface{}) bool {
	if _, ok := v.([]interface{}); ok {
		return true
	} else if _, ok := v.(bson.M); ok {
		return true
	}
	return false
}

func flattenSlice(slice []interface{}, fromOp bool) interface{} {
	var rv []interface{}
	for _, v := range slice {
		if s, ok := v.([]interface{}); ok {
			sv := flattenSlice(s, false)
			if isAggregate(sv) {
				rv = append(rv, sv)
			}
		} else if m, ok := v.(bson.M); ok {
			mv := flattenMap(m, fromOp)
			if isAggregate(mv) || fromOp {
				rv = append(rv, mv)
			}
		}
	}
	// if the slice is empty, return 1 (since it's entirely primitives).
	// otherwise return the slice
	if len(rv) == 0 {
		return 1
	}
	return rv
}

func flattenMap(m bson.M, fromOp bool) interface{} {
	rv := make(bson.M)
	for k, v := range m {
		if strings.HasPrefix(k, "$") {
			rv[k] = flattenOp(v)
		} else {
			flattened := flatten(v)
			if isAggregate(flattened) || fromOp {
				rv[k] = flattened
			}
		}
	}
	// if the slice is empty, return 1 (since it's entirely primitives).
	// otherwise return the slice
	if len(rv) == 0 {
		return 1
	}
	return rv
}

func flatten(v interface{}) interface{} {
	if s, ok := v.([]interface{}); ok {
		return flattenSlice(s, false)
	} else if m, ok := v.(bson.M); ok {
		return flattenMap(m, false)
	} else {
		return 1
	}
}

func flattenOp(v interface{}) interface{} {
	if s, ok := v.([]interface{}); ok {
		return flattenSlice(s, true)
	} else if m, ok := v.(bson.M); ok {
		return flattenMap(m, true)
	} else {
		return 1
	}
}

func serializeShape(shape interface{}) string {
	// we can't just json marshal, since we need ordered keys
	if m, ok := shape.(bson.M); ok {
		var keys []string
		var keyAndVal []string
		for k := range m {
			keys = append(keys, k)
		}

		sort.Strings(keys)
		for _, k := range keys {
			keyAndVal = append(keyAndVal, "\""+k+"\":"+serializeShape(m[k]))
		}

		return "{" + strings.Join(keyAndVal, ",") + "}"

	} else if s, ok := shape.([]interface{}); ok {
		var vals []string
		for _, v := range s {
			vals = append(vals, serializeShape(v))
		}
		return "[" + strings.Join(vals, ",") + "]"
	} else {
		return "1"
	}
}
