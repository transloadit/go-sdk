package transloadit

import (
	"strings"
	"testing"
)

func TestListNotifications(t *testing.T) {
	t.Parallel()

	client := setup(t)
	_, err := client.ListNotifications(ctx, &ListOptions{
		PageSize: 3,
	})

	if err == nil {
		t.Fatal("expected an error but got nil")
	}

	if !strings.Contains(err.Error(), "no longer available") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestReplayNotification(t *testing.T) {
	t.Parallel()

	client := setup(t)

	// Create a Assembly to later replay its notifications
	assembly := NewAssembly()
	assembly.AddFile("image", "./fixtures/lol_cat.jpg")
	assembly.AddStep("resize", map[string]interface{}{
		"robot":  "/image/resize",
		"width":  75,
		"height": 75,
	})
	assembly.NotifyURL = "https://transloadit.com/notify-url/"

	info, err := client.StartAssembly(ctx, assembly)
	if err != nil {
		t.Fatal(err)
	}

	info, err = client.WaitForAssembly(ctx, info)
	if err != nil {
		t.Fatal(err)
	}

	// Test replay notification with custom notify URL
	if err = client.ReplayNotification(ctx, info.AssemblyID, "https://transloadit.com/custom-notify"); err != nil {
		t.Fatal(err)
	}
}
