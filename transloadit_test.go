package transloadit

import "testing"

var (
	apikey = "b3a87350e97a11e380176daba9afca9d"
	secret = "91e76a7b699e5590268917c4bbdc65141471e40b"
)

func TestRequest(t *testing.T) {
	i, err := NewInstance(apikey, secret)
	if err != nil {
		t.Fatal(err)
	}
	//fmt.Print(i)
	p := Params{
		Auth: Auth{
			Key: i.apikey,
		},
		Steps: map[string]interface{}{
			"resize": map[string]interface{}{
				"robot":  "/image/resize",
				"width":  200,
				"height": 200,
				"use":    ":original",
			},
		},
	}
	p.Init()
	//fmt.Print(p)
	filepath := "/Users/slowpoke/Pictures/test/aface.png"
	_, err = i.CreateAssembly(p, filepath)
	if err != nil {
		t.Fatal(err)
	}
}
