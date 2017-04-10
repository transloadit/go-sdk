package transloadit

import (
	"context"
	"time"
)

// NotificationList contains a list of notifications.
type NotificationList struct {
	Notifications []Notification `json:"items"`
	Count         int            `json:"count"`
}

// Notification contains details about a notification.
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

// ListNotification will return a list containing all notifications matching
// the criteria defined using the ListOptions structure.
func (client *Client) ListNotifications(ctx context.Context, options *ListOptions) (list NotificationList, err error) {
	err = client.listRequest(ctx, "assembly_notifications", options, &list)
	return list, err
}

// ReplayNotification instructs the endpoint to replay the notification
// corresponding to the provided assembly ID.
// If notifyUrl is not empty it will override the notify URL used in the
// assembly instructions.
func (client *Client) ReplayNotification(ctx context.Context, assemblyId string, notifyUrl string) error {
	params := make(map[string]interface{})

	if notifyUrl != "" {
		params["notify_url"] = notifyUrl
	}

	return client.request(ctx, "POST", "assembly_notifications/"+assemblyId+"/replay", params, nil)
}
