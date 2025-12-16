module github.com/skullzarmy/porcupin-ipfs-bakup-node

go 1.25

require (
	github.com/wailsapp/wails/v2 v2.8.0
	gorm.io/gorm v1.25.5
	gorm.io/driver/sqlite v1.5.4
)

// Note: Additional dependencies will be added as implementation progresses:
// - github.com/ipfs/kubo (IPFS node)
// - golang.org/x/time/rate (Rate limiting)
// - Database migration tools
