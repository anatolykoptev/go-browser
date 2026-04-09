package browser

import (
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// setupProxyAuth enables continuous Fetch.authRequired handling for proxy authentication.
// Intercepts all requests but immediately continues non-auth ones in goroutines
// to avoid blocking page XHR. Auth challenges get credentials.
// Returns a cleanup function that disables the fetch domain.
func setupProxyAuth(b *rod.Browser, username, password string) func() {
	_ = proto.FetchEnable{
		HandleAuthRequests: true,
	}.Call(b)

	wait := b.EachEvent(
		func(ev *proto.FetchRequestPaused) {
			// Continue immediately in goroutine — don't block the event loop.
			go func() {
				_ = proto.FetchContinueRequest{RequestID: ev.RequestID}.Call(b)
			}()
		},
		func(ev *proto.FetchAuthRequired) {
			go func() {
				_ = proto.FetchContinueWithAuth{
					RequestID: ev.RequestID,
					AuthChallengeResponse: &proto.FetchAuthChallengeResponse{
						Response: proto.FetchAuthChallengeResponseResponseProvideCredentials,
						Username: username,
						Password: password,
					},
				}.Call(b)
			}()
		},
	)

	go wait()

	return func() {
		_ = proto.FetchDisable{}.Call(b)
	}
}
