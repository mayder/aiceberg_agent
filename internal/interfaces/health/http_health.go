package health

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/you/aiceberg_agent/internal/common/logger"
)

func Serve(port int, log logger.Logger) {
	addr := ":" + strconv.Itoa(port)
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "ok")
	})
	log.Info("health on " + addr)
	_ = http.ListenAndServe(addr, nil)
}
