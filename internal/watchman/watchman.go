package watchman

import "github.com/fsnotify/fsnotify"

type Watcher struct {
	*fsnotify.Watcher
}

func New() (*Watcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	return &Watcher{w}, nil
}

func (w *Watcher) AddDir(path string) error {
	return w.Add(path)
}
