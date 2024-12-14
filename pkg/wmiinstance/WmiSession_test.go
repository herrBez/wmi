package cim

import (
	"testing"
	"time"
)

type myTestCallbackContext struct {
	test       *testing.T
	counter    int
	invokation int
	completed  chan EventType
}

type EventType int64

const (
	READY EventType = iota
	COMPLETED
	PROGRESS
	PUT
)

func myOnObjectReady(context interface{}, wmiInstances []*WmiInstance) {
	t := context.(*myTestCallbackContext)
	// t.test.Logf("On Object Ready: Fetched %d %v", len(wmiInstances))
	t.invokation += 1
	t.counter += len(wmiInstances)

	t.completed <- READY
}

func myOnCompleted(context interface{}, wmiInstances []*WmiInstance) {
	t := context.(*myTestCallbackContext)
	t.test.Logf("On Completed: Fetched %d", len(wmiInstances))
	t.completed <- COMPLETED
}

func myOnProgress(context interface{}, wmiInstances []*WmiInstance) {
	t := context.(*myTestCallbackContext)
	t.test.Log("On Progress")
	// t.completed <- true
}

func myOnObjectPut(context interface{}, wmiInstances []*WmiInstance) {
	t := context.(*myTestCallbackContext)
	t.test.Log("On Object Put")

	// t.completed <- true
}

func Test_PerfromRawAsyncQuery(t *testing.T) {

	t.Logf("Starting the Test")
	sessionManager := NewWmiSessionManager()
	defer sessionManager.Dispose()

	session, err := sessionManager.GetLocalSession("ROOT\\CimV2")

	if err != nil {
		t.Errorf("sessionManager.GetSession failed with error %v", err)
		return
	}

	connected, err := session.Connect()

	if !connected || err != nil {
		t.Errorf("session.Connect failed with error %v", err)
		return
	}
	defer session.Close()

	context := myTestCallbackContext{
		test:      t,
		completed: make(chan EventType),
	}
	eventSink, err := CreateWmiEventSink(session, &context, myOnObjectReady, myOnCompleted, myOnProgress, myOnObjectPut)
	if err != nil {
		t.Errorf("CreateWmiEventSink failed with error '%v'", err)
		return
	}
	// defer eventSink.Close()

	_, err = eventSink.Connect()
	if err != nil {
		t.Errorf("Connect failed with error '%v'", err)
		return
	}

	// There is no way of setting Batching parameters, apparently, meaning that every element is returned by OnObjectPut
	//  https://learn.microsoft.com/en-us/windows/win32/api/wbemprov/nf-wbemprov-iwbemeventsink-setbatchingparameters there is no "SWbem" equivalent on which go-ole is based
	//
	// _, err = eventSink.unknown.CallMethod("SetBatchingParameters", 0x1, 10)
	// if err != nil {
	// 	t.Errorf("Batching could not be set %v", err)
	// }

	_, err = session.PerformRawAsyncQuery(eventSink, "SELECT * FROM Win32_ClassicCOMClassSettings")
	if err != nil {
		t.Errorf("CallMethod failed with error '%v'", err)
		return
	}

	timeout := 500 * time.Millisecond

	completed := false
	startTime := time.Now()

	for time.Since(startTime) < timeout && !completed {
		select {
		case value, ok := <-context.completed:
			if !ok {
				t.Errorf("Channel closed, exiting")
				return
			}
			switch value {
			case READY:
				// context.counter++
				// t.Log("A line is ready")
			case COMPLETED:
				t.Log("Completed")

				completed = true

			default:
				t.Errorf("Unexpected Value")
			}

		// Timeout: never received an event from the eventSink
		case <-time.After(timeout):
			t.Log("Timeout Reached")
			completed = true
			// The Cancel Method does not seem to work. It hangs indefenitely
			// _, err = eventSink.instance.CallMethod("Cancel")
			// if err != nil {
			// 	t.Errorf("Could not cancel %v", err)
			// } else {
			// 	t.Log("Cancel After timeout!")
			// }
		}
		for eventSink.PeekAndDispatchMessages() {
			// Continue pumping for message while they arrive
		}
	}

	t.Log("Exit from Main Loop")

	// Let's do manually the event Sink release, because it hangs
	eventSink.unknown.Release()
	// !!!! This is the most important line!!!! Without this it hangs indefintely
	release(eventSink.unknown)
	t.Log("Unknonw Released")

	t.Log("Exit EventSing")
	eventSink.instance.Release()
	t.Log("Exit Release")

	eventSink.Close()

	// Without timeout the elements should be 6k in my system, we fetch ~800 elements and give up
	t.Logf("Fetched %d Elements Asyncrhonously with %d invokations", context.counter, context.invokation)
}
