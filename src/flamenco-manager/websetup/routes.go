package websetup

import (
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
)

func httpSetupIndex(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, "Flamenco Manager Setup")
}

// addWebSetupRoutes registers HTTP endpoints for setup mode.
func addWebSetupRoutes(router *mux.Router) {
	router.HandleFunc("/setup", httpSetupIndex)
}
