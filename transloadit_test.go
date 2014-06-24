package transloadit

import "testing"

var (
	apikey = "foo"
	secret = "bar"
)

func TestRequest(t *testing.T) {
	i, err := NewInstance(apikey, secret)
	if err != nil {
		t.Fatal(err)
	}
	p := Params{
		Auth: Auth{
			Key: i.apikey,
			//Expires: "+2 hours",
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
	filepath := "/Users/slowpoke/Pictures/aface.png"
	_, err = i.SendRequest(p, filepath)
	if err != nil {
		t.Fatal(err)
	}
}
