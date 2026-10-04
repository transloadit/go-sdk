package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/transloadit/go-sdk"
)

// This example starts an HTTP server that receives Assembly Notifications.
// Configure your assembly's NotifyURL (see examples/image-resize) to point
// at this server's /webhook path to try it out.
func main() {
	authSecret := "TRANSLOADIT_SECRET"

	http.HandleFunc("/webhook", func(w http.ResponseWriter, r *http.Request) {
		notification, err := transloadit.ParseAssemblyNotification(r, authSecret)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if notification.Error != "" {
			fmt.Printf("assembly %s failed: %s\n", notification.AssemblyID, notification.Error)
		} else {
			fmt.Printf("assembly %s finished with status %s\n", notification.AssemblyID, notification.Ok)
		}

		w.WriteHeader(http.StatusOK)
	})

	log.Fatal(http.ListenAndServe(":8080", nil))
}
