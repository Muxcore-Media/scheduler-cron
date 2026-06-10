module github.com/Muxcore-Media/scheduler-cron

go 1.26.4

replace github.com/Muxcore-Media/core => /home/enderk/claude/core

require (
	github.com/Muxcore-Media/core v0.4.0
	github.com/robfig/cron/v3 v3.0.1
	google.golang.org/grpc v1.81.1
)

require (
	golang.org/x/net v0.55.0 // indirect
	golang.org/x/sys v0.45.0 // indirect
	golang.org/x/text v0.37.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260226221140-a57be14db171 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)
