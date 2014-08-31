package transloadit

import (
	"encoding/json"
	"fmt"
	"time"
)

type NotificationList struct {
	Notifications []*NotificationListItem `json:"items"`
	Count         int                     `json:"count"`
}

type NotificationListItem struct {
	Id           string    `json:"id"`
	AccountId    string    `json:"account_id"`
	Url          string    `json:"url"`
	ResponseCode int       `json:"response_code"`
	RespandeData string    `json:"response_data"`
	Duration     float32   `json:"duration"`
	Created      time.Time `json:"created"`
	Error        string    `json:"error"`
}

func (client *Client) ListNotifications(options *ListOptions) (*NotificationList, error) {

	body, err := client.listRequest("assembly_notifications", options)
	if err != nil {
		return nil, fmt.Errorf("unable to list notification: %s", err)
	}

	var notification NotificationList
	err = json.Unmarshal(body, &notification)
	if err != nil {
		return nil, fmt.Errorf("unable to list notification: %s", err)
	}

	return &notification, nil
}
