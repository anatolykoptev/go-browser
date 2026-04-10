package browser

import "fmt"

func init() {
	registerAction("get_storage", execGetStorage)
	registerAction("set_storage", execSetStorage)
	registerAction("clear_storage", execClearStorage)
}

func storageObj(storageType string) string {
	if storageType == "session" {
		return "sessionStorage"
	}
	return "localStorage"
}

func execGetStorage(dc dispatchContext, a Action) (any, error) {
	obj := storageObj(a.StorageType)
	if a.Key != "" {
		script := fmt.Sprintf(`%s.getItem(%q)`, obj, a.Key)
		return doEvaluate(dc.page, script)
	}
	script := fmt.Sprintf(`JSON.stringify(Object.fromEntries(Object.entries(%s)))`, obj)
	return doEvaluate(dc.page, script)
}

func execSetStorage(dc dispatchContext, a Action) (any, error) {
	obj := storageObj(a.StorageType)
	if a.Key == "" {
		return nil, fmt.Errorf("set_storage: key is required")
	}
	script := fmt.Sprintf(`%s.setItem(%q, %q)`, obj, a.Key, a.Text)
	_, err := doEvaluate(dc.page, script)
	return nil, err
}

func execClearStorage(dc dispatchContext, a Action) (any, error) {
	obj := storageObj(a.StorageType)
	if a.Key != "" {
		script := fmt.Sprintf(`%s.removeItem(%q)`, obj, a.Key)
		_, err := doEvaluate(dc.page, script)
		return nil, err
	}
	script := fmt.Sprintf(`%s.clear()`, obj)
	_, err := doEvaluate(dc.page, script)
	return nil, err
}
