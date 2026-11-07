package conf

import (
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

func (p *Conf) Watch(filePath, xDnsPath string, sDnsPath string, reload func()) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("new watcher error: %s", err)
	}
	for _, p := range []string{filePath, xDnsPath, sDnsPath} {
		if p == "" {
			continue
		}
		if err := watcher.Add(p); err != nil {
			watcher.Close()
			return fmt.Errorf("watch file error: %s", err)
		}
	}
	go func() {
		var pre time.Time
		defer watcher.Close()
		for {
			select {
			case e, ok := <-watcher.Events:
				if !ok {
					return
				}
				if e.Has(fsnotify.Chmod) {
					continue
				}
				if pre.Add(10 * time.Second).After(time.Now()) {
					continue
				}
				pre = time.Now()
				go func() {
					time.Sleep(5 * time.Second)
					switch filepath.Base(strings.TrimSuffix(e.Name, "~")) {
					case filepath.Base(xDnsPath), filepath.Base(sDnsPath):
						log.Println("DNS file changed, reloading...")
					default:
						log.Println("config file changed, reloading...")
					}
					next := New()
					err := next.LoadFromPath(filePath)
					if err == nil {
						err = next.Validate()
					}
					if err != nil {
						log.Printf("reload config error: %s", err)
						return
					}
					*p = *next
					reload()
					log.Println("reload config success")
				}()
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				if err != nil {
					log.Printf("File watcher error: %s", err)
				}
			}
		}
	}()
	return nil
}
