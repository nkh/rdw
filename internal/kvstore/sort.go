package kvstore

import "sort"

func sortKeys(keys []Key) {
	sort.Slice(keys, func(i, j int) bool {
		return keys[i] < keys[j]
	})
}
