package auxiliary

import "testing"

func TestForkInheritsPreferencesAndTitleWithoutSharingFutureChanges(t *testing.T) {
	controller, err := New(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer controller.Close()
	if err = controller.SetSession("parent", "summary", false); err != nil {
		t.Fatal(err)
	}
	if err = controller.SetTitle("parent", "Current title"); err != nil {
		t.Fatal(err)
	}
	if err = controller.InheritSession("parent", "child"); err != nil {
		t.Fatal(err)
	}
	title, err := controller.Title("child")
	if err != nil || title.Text != "Current title" || !title.Manual || title.Job != "" {
		t.Fatal("title not inherited", title, err)
	}
	preferences, err := controller.SessionConfig("child")
	if err != nil || preferences.Summary {
		t.Fatal("preferences not inherited", preferences, err)
	}
	if err = controller.SetTitle("child", "Independent child"); err != nil {
		t.Fatal(err)
	}
	if err = controller.InheritSession("parent", "child"); err != nil {
		t.Fatal(err)
	}
	title, _ = controller.Title("child")
	parent, _ := controller.Title("parent")
	if title.Text != "Independent child" || parent.Text != "Current title" {
		t.Fatal("retry overwrote independent metadata", title, parent)
	}
}
