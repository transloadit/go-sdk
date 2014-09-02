package transloadit

import (
	"time"
)

type NotificationList struct {
	Notifications []*NotificationListItem `json:"items"`
	Count         int                     `json:"count"`
}

type NotificationListItem struct {
	Id           string    `json:"id"`
	AssemblyId   string    `json:"assembly_id"`
	AccountId    string    `json:"account_id"`
	Url          string    `json:"url"`
	ResponseCode int       `json:"response_code"`
	RespandeData string    `json:"response_data"`
	Duration     float32   `json:"duration"`
	Created      time.Time `json:"created"`
	Error        string    `json:"error"`
}

// List all notificaions matching the criterias.
func (client *Client) ListNotifications(options *ListOptions) (*NotificationList, error) {

	var notifications NotificationList
	_, err := client.listRequest("assembly_notifications", options, &notifications)
	return &notifications, err

}

// Replay a notification which was trigger by assembly defined using the assemblyId.
// If notifyUrl is not empty it will override the original notify url.
func (client *Client) ReplayNotification(assemblyId string, notifyUrl string) (Response, error) {

	params := make(map[string]interface{})

	if notifyUrl != "" {
		params["notify_url"] = notifyUrl
	}

	return client.request("POST", "assembly_notifications/"+assemblyId+"/replay", params, nil)

}
