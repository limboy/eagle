package main

import (
	"log"

	"github.com/egoist/mygo"
)

// Command is a menu command the page carries out, such as "addFolder".
var Command = mygo.NewEvent[string]("command")

func command(name string) func(*mygo.MenuItem, *mygo.Window) {
	return func(_ *mygo.MenuItem, _ *mygo.Window) { _ = Command.Broadcast(name) }
}

func menu() *mygo.Menu {
	return mygo.NewMenu([]*mygo.MenuItem{
		{Label: "Eagle", Submenu: []*mygo.MenuItem{
			{Role: mygo.RoleAbout},
			mygo.Separator(),
			{Label: "Settings…", Accelerator: "CmdOrCtrl+,", Click: command("settings")},
			mygo.Separator(),
			{Role: mygo.RoleServices},
			mygo.Separator(),
			{Role: mygo.RoleHide},
			{Role: mygo.RoleHideOthers},
			{Role: mygo.RoleUnhide},
			mygo.Separator(),
			{Role: mygo.RoleQuit},
		}},
		{Label: "File", Submenu: []*mygo.MenuItem{
			{Label: "Add Folder…", Accelerator: "CmdOrCtrl+O", Click: command("addFolder")},
			mygo.Separator(),
			{Label: "Open in Viewer", Accelerator: "CmdOrCtrl+Down", Click: command("openViewer")},
			{Label: "Quick Look", Accelerator: "CmdOrCtrl+Y", Click: command("quickLook")},
			{Label: "Show in Finder", Accelerator: "CmdOrCtrl+Shift+R", Click: command("reveal")},
			mygo.Separator(),
			{Label: "Move to Trash", Accelerator: "CmdOrCtrl+Backspace", Click: command("trash")},
			mygo.Separator(),
			{Label: "Describe with AI", Accelerator: "CmdOrCtrl+Shift+D", Click: command("describe")},
			mygo.Separator(),
			{Role: mygo.RoleClose},
		}},
		{Role: mygo.RoleEditMenu},
		{Label: "View", Submenu: []*mygo.MenuItem{
			{Label: "Justified", Accelerator: "CmdOrCtrl+1", Click: command("layoutJustified")},
			{Label: "Grid", Accelerator: "CmdOrCtrl+2", Click: command("layoutGrid")},
			mygo.Separator(),
			{Label: "Larger Thumbnails", Accelerator: "CmdOrCtrl+=", Click: command("zoomIn")},
			{Label: "Smaller Thumbnails", Accelerator: "CmdOrCtrl+-", Click: command("zoomOut")},
			mygo.Separator(),
			{Label: "Show Filters", Accelerator: "CmdOrCtrl+Shift+F", Click: command("toggleFilters")},
			{Label: "Show Inspector", Accelerator: "CmdOrCtrl+I", Click: command("toggleInspector")},
			mygo.Separator(),
			{Label: "Find", Accelerator: "CmdOrCtrl+F", Click: command("find")},
			mygo.Separator(),
			{Role: mygo.RoleToggleFullScreen},
			{Role: mygo.RoleToggleDevTools},
		}},
		{Role: mygo.RoleWindowMenu},
	})
}

func main() {
	lib := newLibrary()
	mygo.Bind(lib)
	mygo.Bind(lib.ai)
	if err := mygo.Protocol.Handle("eagle", mediaHandler{lib}); err != nil {
		log.Fatal(err)
	}

	mygo.App.WhenReady(func() {
		lib.start()
		mygo.App.SetMenu(menu())
		mygo.NewWindow(mygo.WindowOptions{
			Title:     "Eagle",
			Width:     1280,
			Height:    820,
			MinWidth:  860,
			MinHeight: 520,
			// The sidebar runs up to the top, under the traffic lights,
			// over the system's translucent sidebar material.
			TitleBarStyle:   mygo.TitleBarHiddenInset,
			TitleBarHeight:  52,
			Transparent:     true,
			Vibrancy:        mygo.VibrancySidebar,
			BackgroundColor: "light-dark(#ffffff, #1b1b1d)",
			StateKey:        "main",
			URL:             "/",
		})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
