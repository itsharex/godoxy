package middleware

import (
	"maps"
	"net/http"
	"strconv"

	gperr "github.com/yusing/goutils/errs"
)

type middlewareChain struct {
	befores  []RequestModifier
	modResps []ResponseModifier
}

// TODO: check conflict or duplicates.
func NewMiddlewareChain(name string, chain []*Middleware) *Middleware {
	chainMid := &middlewareChain{}
	m := &Middleware{name: name, impl: chainMid}

	for _, comp := range chain {
		if before, ok := comp.impl.(RequestModifier); ok {
			chainMid.befores = append(chainMid.befores, before)
		}
		if mr, ok := comp.impl.(ResponseModifier); ok {
			chainMid.modResps = append(chainMid.modResps, mr)
		}
	}
	return m
}

// before implements RequestModifier.
func (m *middlewareChain) before(w http.ResponseWriter, r *http.Request) (proceedNext bool) {
	if len(m.befores) == 0 {
		return true
	}
	for _, b := range m.befores {
		if proceedNext = b.before(w, r); !proceedNext {
			return false
		}
	}
	return true
}

// modifyResponse implements ResponseModifier.
func (m *middlewareChain) modifyResponse(resp *http.Response) error {
	if len(m.modResps) == 0 {
		return nil
	}
	allowBodyModification := canModifyResponseBody(resp)
	for i, mr := range m.modResps {
		respToModify := resp
		if !allowBodyModification {
			shadow := *resp
			shadow.Body = eofReader{}
			respToModify = &shadow
		}
		if err := mr.modifyResponse(respToModify); err != nil {
			return gperr.PrependSubject(err, strconv.Itoa(i))
		}
		if !allowBodyModification {
			resp.StatusCode = respToModify.StatusCode
			if respToModify.Header != nil {
				maps.Copy(resp.Header, respToModify.Header)
			}
		}
	}
	return nil
}
