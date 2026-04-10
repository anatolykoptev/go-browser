package browser

import "testing"

func TestStorageActionTypes_Registered(t *testing.T) {
	for _, name := range []string{"get_storage", "set_storage", "clear_storage"} {
		if _, ok := actionRegistry[name]; !ok {
			t.Errorf("action %q not registered", name)
		}
	}
}
