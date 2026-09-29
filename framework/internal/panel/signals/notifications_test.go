package signals

import "testing"

// C1: every documented notification type is mapped explicitly.
func TestNotificationTableCoversEveryDocumentedType(t *testing.T) {
	t.Parallel()
	if n := len(NotificationTypes()); n != 12 {
		t.Fatalf("want the 12 documented types, got %d", n)
	}
	for _, typ := range NotificationTypes() {
		k := ClassifyNotification(typ)
		if !k.Known || k.Label == "" || k.Severity == "" {
			t.Errorf("%s is not mapped: %+v", typ, k)
		}
	}
	if len(notificationTable) != 12 {
		t.Errorf("the table maps %d types; a new one needs a deliberate row and a test", len(notificationTable))
	}
}
