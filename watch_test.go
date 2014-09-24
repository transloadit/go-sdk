package transloadit

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestWatch(t *testing.T) {

	// Clean up testing environment
	remove("./fixtures/input/lol_cat.jpg")
	remove("./fixtures/input/mona_lisa.jpg")
	remove("./fixtures/output/resize_0_lol_cat.jpg")
	remove("./fixtures/output/-original_0_lol_cat.jpg")
	remove("./fixtures/output/resize_0_mona_lisa.jpg")
	remove("./fixtures/output/-original_0_mona_lisa.jpg")
	copyFile("./fixtures/lol_cat.jpg", "./fixtures/input/lol_cat.jpg")

	client := setup(t)

	options := &WatchOptions{
		TemplateId: "68302e702fbd11e4b5fb01f025693443",
		Input:      "./fixtures/input",
		Output:     "./fixtures/output",
		Steps:      make(map[string]map[string]interface{}),
		Watch:      true,
		Preserve:   true,
	}

	watcher := client.Watch(options)
	fmt.Println(1)
	go func() {
		err, more := <-watcher.Error
		if !more {
			return
		}
		t.Fatal(err)
	}()
	fmt.Println(2)
	info := <-watcher.Done
	if info.Uploads[0].Name != "lol_cat.jpg" {
		t.Fatal("wrong file uploaded")
	}
	fmt.Println(3)
	if !exists("./fixtures/output/resize_0_lol_cat.jpg") {
		t.Fatal("output file resize_0_lol_cat.jpg not created")
	}
	fmt.Println(4)
	if !exists("./fixtures/output/-original_0_lol_cat.jpg") {
		t.Fatal("output file -original_0_lol_cat.jpg not created")
	}
	fmt.Println(5)
	if exists("./fixtures/input/lol_cat.jpg") {
		t.Fatal("output file lol_cat.jpg not deleted")
	}
	fmt.Println(6)
	go copyFile("./fixtures/mona_lisa.jpg", "./fixtures/input/mona_lisa.jpg")
	fmt.Println(7)
	changedFile := <-watcher.Change
	if filepath.ToSlash(changedFile) != "fixtures/input/mona_lisa.jpg" {
		t.Fatal("wrong changed file name")
	}
	fmt.Println(8)
	info = <-watcher.Done
	if info.Uploads[0].Name != "mona_lisa.jpg" {
		t.Fatal("wrong file uploaded")
	}
	fmt.Println(9)
	if !exists("./fixtures/output/resize_0_mona_lisa.jpg") {
		t.Fatal("output file resize_0_mona_lisa.jpg not created")
	}
	fmt.Println(10)
	if !exists("./fixtures/output/-original_0_mona_lisa.jpg") {
		t.Fatal("output file -original_0_mona_lisa.jpg not created")
	}
	fmt.Println(11)
	if exists("./fixtures/input/mona_lisa.jpg") {
		t.Fatal("output file mona_lisa.jpg not deleted")
	}
	fmt.Println(12)
	watcher.Stop()
	fmt.Println(13)
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

func remove(filename string) {

	if err := os.RemoveAll(filename); err != nil {
		panic(err)
	}

}
