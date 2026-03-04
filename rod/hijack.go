package rod

import (
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// blockResources sets up request interception to abort matching resource types.
// Returns a cleanup function that stops the hijack router.
func blockResources(page *rod.Page, types []ResourceType) func() {
	if len(types) == 0 || page == nil {
		return func() {}
	}

	blocked := make(map[proto.NetworkResourceType]bool, len(types))
	for _, t := range types {
		blocked[proto.NetworkResourceType(t)] = true
	}

	router := page.HijackRequests()
	router.MustAdd("*", func(ctx *rod.Hijack) {
		if blocked[ctx.Request.Type()] {
			ctx.Response.Fail(proto.NetworkErrorReasonBlockedByClient)
			return
		}
		ctx.ContinueRequest(&proto.FetchContinueRequest{})
	})
	go router.Run()

	return func() { _ = router.Stop() }
}
