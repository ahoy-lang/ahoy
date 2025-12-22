module ahoy/source

go 1.25

require (
	ahoy v0.0.0
	github.com/fsnotify/fsnotify v1.7.0
)

require golang.org/x/sys v0.4.0 // indirect

replace ahoy => ../pkg
