package lweixin

import (
	"errors"
	"testing"

	"github.com/multica-ai/multica/server/pkg/dbid"
)

func TestRouteAgentInput(t *testing.T) {
	id := dbid.NewV7()
	value := uuidPointer(id)
	for _, tc := range []struct {
		choice RouteChoice
		valid  bool
	}{
		{RouteChoice{Mode: "inherit"}, true},
		{RouteChoice{Mode: "silent"}, true},
		{RouteChoice{Mode: "agent", AgentID: value}, true},
		{RouteChoice{Mode: "agent"}, false},
		{RouteChoice{Mode: "inherit", AgentID: value}, false},
		{RouteChoice{Mode: "silent", AgentID: value}, false},
		{RouteChoice{Mode: "other"}, false},
	} {
		_, err := routeAgent(tc.choice)
		if (err == nil) != tc.valid || (err != nil && !errors.Is(err, ErrRouteInvalid)) {
			t.Fatalf("route %+v: %v", tc.choice, err)
		}
	}
}
