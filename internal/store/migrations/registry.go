package migrations

import "fmt"

const MinSupportedSchema = 1

type Migration struct {
	Version   int
	UpDB      string
	UpPayload func(m map[string]any)
}

var all = []Migration{
	V1,
}

func All() []Migration {
	return append([]Migration(nil), all...)
}

func Latest() int {
	return all[len(all)-1].Version
}

func ByVersion(v int) (Migration, bool) {
	for _, m := range all {
		if m.Version == v {
			return m, true
		}
	}
	return Migration{}, false
}

func init() {
	if len(all) == 0 {
		panic("migrations: empty chain")
	}
	for i, m := range all {
		if want := MinSupportedSchema + i; m.Version != want {
			panic(fmt.Sprintf("migrations: entry %d is properties %d, want %d",
				i, m.Version, want))
		}
	}
}
