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
	ID           string    `json:"id"`
	AssemblyID   string    `json:"assembly_id"`
	AccountID    string    `json:"account_id"`
	URL          string    `json:"url"`
	ResponseCode int       `json:"response_code"`
	ResponseData string    `json:"response_data"`
	Duration     float32   `json:"duration"`
	Created      time.Time `json:"created"`
	Error        string    `json:"error"`
}

// ListNotifications will return a list containing all notifications matching
// the criteria defined using the ListOptions structure.
func (client *Client) ListNotifications(ctx context.Context, options *ListOptions) (list NotificationList, err error) {
	err = client.listRequest(ctx, "assembly_notifications", options, &list)
	return list, err
}

// ReplayNotification instructs the endpoint to replay the notification
// corresponding to the provided assembly ID.
// If notifyURL is not empty it will override the notify URL used in the
// assembly instructions.
func (client *Client) ReplayNotification(ctx context.Context, assemblyID string, notifyURL string) error {
	params := make(map[string]interface{})

	if notifyURL != "" {
		params["notify_url"] = notifyURL
	}

	return client.request(ctx, "POST", "assembly_notifications/"+assemblyID+"/replay", params, nil)
}
