package thumbnailer

import "testing"

func TestSetFFmpegLogLevel(t *testing.T) {
	tests := []struct {
		name     string
		logLevel AVLogLevel
	}{
		{"Quiet", AVLogQuiet},
		{"Panic", AVLogPanic},
		{"Fatal", AVLogFatal},
		{"Error", AVLogError},
		{"Warning", AVLogWarning},
		{"Info", AVLogInfo},
		{"Verbose", AVLogVerbose},
		{"Debug", AVLogDebug},
		{"Trace", AVLogTrace},
	}
	level := logLevel()
	if level != AVLogInfo {
		t.Logf("AVLogLevel = %v", level)
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			SetFFmpegLogLevel(test.logLevel)
			level := logLevel()
			if level != test.logLevel {
				t.Errorf("AVLogLevel want = %v, got = %v", test.logLevel, level)
			}
		})
	}
	SetFFmpegLogLevel(AVLogInfo)
}
