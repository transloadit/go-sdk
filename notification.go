package transloadit

import (
	"time"
)

type NotificationList struct {
	Notifications []Notification `json:"items"`
	Count         int            `json:"count"`
}

type Notification struct {
	Id           string    `json:"id"`
	AssemblyId   string    `json:"assembly_id"`
	AccountId    string    `json:"account_id"`
	Url          string    `json:"url"`
	ResponseCode int       `json:"response_code"`
	ResponseData string    `json:"response_data"`
	Duration     float32   `json:"duration"`
	Created      time.Time `json:"created"`
	Error        string    `json:"error"`
}

// ListNotification will return a slice containing all notifications matching
// the criteria defined using the ListOptions structure.
func (client *Client) ListNotifications(options *ListOptions) (list NotificationList, err error) {
	err = client.listRequest("assembly_notifications", options, &list)
	return list, err
}

// Replay a notification which was trigger by assembly defined using the assemblyId.
// If notifyUrl is not empty it will override the original notify url.
func (client *Client) ReplayNotification(assemblyId string, notifyUrl string) error {
	params := make(map[string]interface{})

	if notifyUrl != "" {
		params["notify_url"] = notifyUrl
	}

	return client.request("POST", "assembly_notifications/"+assemblyId+"/replay", params, nil)
}
