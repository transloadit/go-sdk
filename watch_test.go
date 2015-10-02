package transloadit

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestWatch(t *testing.T) {
	// Clean up testing environment
	emptyDir("./fixtures/input/")
	emptyDir("./fixtures/output")
	copyFile("./fixtures/lol_cat.jpg", "./fixtures/input/lol_cat.jpg")

	setupTemplates(t)
	client := setup(t)

	options := &WatchOptions{
		TemplateId: templateIdOptimizeResize,
		Input:      "./fixtures/input",
		Output:     "./fixtures/output",
		Steps:      make(map[string]map[string]interface{}),
		Watch:      true,
		Preserve:   true,
	}

	watcher := client.Watch(options)

	go func() {
		err, more := <-watcher.Error
		if !more {
			return
		}
		t.Fatal(err)
	}()

	info := <-watcher.Done
	if info.Uploads[0].Name != "lol_cat.jpg" {
		t.Fatal("wrong file uploaded")
	}

	if !exists("./fixtures/output/image-resize_0_lol_cat.jpg") {
		t.Fatal("output file image-resize_0_lol_cat.jpg not created")
	}

	if !exists("./fixtures/output/-original_0_lol_cat.jpg") {
		t.Fatal("output file -original_0_lol_cat.jpg not created")
	}

	if exists("./fixtures/input/lol_cat.jpg") {
		t.Fatal("output file lol_cat.jpg not deleted")
	}

	go copyFile("./fixtures/mona_lisa.jpg", "./fixtures/input/mona_lisa.jpg")

	changedFile := <-watcher.Change
	moneLisa, err := filepath.Abs("./fixtures/input/mona_lisa.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.ToSlash(changedFile) != moneLisa {
		t.Fatal("wrong changed file name")
	}

	info = <-watcher.Done
	if info.Uploads[0].Name != "mona_lisa.jpg" {
		t.Fatal("wrong file uploaded")
	}

	if !exists("./fixtures/output/image-resize_0_mona_lisa.jpg") {
		t.Fatal("output file image-resize_0_mona_lisa.jpg not created")
	}

	if !exists("./fixtures/output/-original_0_mona_lisa.jpg") {
		t.Fatal("output file -original_0_mona_lisa.jpg not created")
	}

	if exists("./fixtures/input/mona_lisa.jpg") {
		t.Fatal("output file mona_lisa.jpg not deleted")
	}

	watcher.Stop()
}

func exists(filename string) bool {
	_, err := os.Stat(filename)
	if err == nil {
		return true
	}
	if os.IsNotExist(err) {
		return false
	}
	panic(err)
}

func copyFile(src, dst string) {
	in, err := os.Open(src)
	if err != nil {
		panic(err)
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		panic(err)
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	if err != nil {
		panic(err)
	}
}

func emptyDir(path string) {
	if err := os.RemoveAll(path); err != nil {
		panic(err)
	}

	if err := os.MkdirAll(path, os.ModePerm); err != nil {
		panic(err)
	}
}
