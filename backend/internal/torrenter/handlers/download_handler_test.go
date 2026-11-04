package handlers

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The webserver dropping its connection (a restart, a timeout) must not cancel
// the search and torrent add running for it: the work context keeps the
// request's values but not its cancellation.
func TestWorkContext_SurvivesClientDisconnect(t *testing.T) {
	gin.SetMode(gin.TestMode)
	reqCtx, disconnect := context.WithCancel(context.Background())
	type key struct{}
	reqCtx = context.WithValue(reqCtx, key{}, "trace")
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/download", nil).WithContext(reqCtx)

	ctx := workContext(c)
	disconnect()

	require.Error(t, reqCtx.Err(), "the request context is cancelled")
	assert.NoError(t, ctx.Err(), "the work context is not")
	assert.Equal(t, "trace", ctx.Value(key{}), "values such as the trace carry over")
}
