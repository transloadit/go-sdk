package transloadit

import (
	"testing"
)

func TestListNotifications(t *testing.T) {
	client := setup(t)

	notification, err := client.ListNotifications(ctx, &ListOptions{
		PageSize: 3,
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(notification.Notifications) < 3 {
		t.Fatal("wrong number of notification")
	}

	if notification.Count == 0 {
		t.Fatal("wrong count")
	}

	if notification.Notifications[0].ID == "" {
		t.Fatal("wrong notification name")
	}
}
