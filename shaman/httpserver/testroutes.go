package httpserver

/* ***** BEGIN GPL LICENSE BLOCK *****
 *
 * This program is free software; you can redistribute it and/or
 * modify it under the terms of the GNU General Public License
 * as published by the Free Software Foundation; either version 2
 * of the License, or (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program; if not, write to the Free Software Foundation,
 * Inc., 59 Temple Place - Suite 330, Boston, MA  02111-1307, USA.
 *
 * ***** END GPL LICENCE BLOCK *****
 *
 * (c) 2019, Blender Foundation - Sybren A. Stüvel
 */

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/armadillica/flamenco-manager/shaman/auth"
	"github.com/gorilla/mux"
)

// Product is our test API payload
type Product struct {
	ID          int
	Name        string
	Slug        string
	Description string
}

var products = []Product{
	Product{ID: 1, Name: "Hover Shooters", Slug: "hover-shooters", Description: "Shoot your way to the top on 14 different hoverboards"},
	Product{ID: 2, Name: "Ocean Explorer", Slug: "ocean-explorer", Description: "Explore the depths of the sea in this one of a kind underwater experience"},
	Product{ID: 3, Name: "Dinosaur Park", Slug: "dinosaur-park", Description: "Go back 65 million years in the past and ride a T-Rex"},
	Product{ID: 4, Name: "Cars VR", Slug: "cars-vr", Description: "Get behind the wheel of the fastest cars in the world."},
	Product{ID: 5, Name: "Robin Hood", Slug: "robin-hood", Description: "Pick up the bow and arrow and master the art of archery"},
	Product{ID: 6, Name: "Real World VR", Slug: "real-world-vr", Description: "Explore the seven wonders of the world in VR"},
}

// Here we are implementing the notImplemented handler. Whenever an API endpoint is hit
// we will simply return the message "Not Implemented"
var notImplemented = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
	w.Write([]byte("Not Implemented"))
})

/* The status handler will be invoked when the user calls the /status route
It will simply return a string with the message "API is up and running" */
var statusHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("API is up and running"))
})

/* The products handler will be called when the user makes a GET request to the /products endpoint.
This handler will return a list of products available for users to review */
var productsHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	// Here we are converting the slice of products to JSON
	payload, _ := json.Marshal(products)

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(payload))
})

/* The feedback handler will add either positive or negative feedback to the product
We would normally save this data to the database - but for this demo, we'll fake it
so that as long as the request is successful and we can match a product to our catalog of products
we'll return an OK status. */
var addFeedbackHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	var product Product
	vars := mux.Vars(r)
	slug := vars["slug"]

	for _, p := range products {
		if p.Slug == slug {
			product = p
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if product.Slug != "" {
		payload, _ := json.Marshal(product)
		w.Write([]byte(payload))
	} else {
		w.Write([]byte("Product Not Found"))
	}
})

// RegisterTestRoutes registers some routes that should only be used for testing.
func RegisterTestRoutes(r *mux.Router, auther auth.Authenticator) {
	// On the default page we will simply serve our static index page.
	r.Handle("/", http.FileServer(http.Dir("./views/")))

	// We will setup our server so we can serve static assest like images, css from the /static/{file} route
	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir("./static/"))))

	// Our API is going to consist of three routes
	// /status - which we will call to make sure that our API is up and running
	// /products - which will retrieve a list of products that the user can leave feedback on
	// /products/{slug}/feedback - which will capture user feedback on products
	r.Handle("/status", statusHandler).Methods("GET")
	r.Handle("/products", auther.Wrap(productsHandler)).Methods("GET")
	r.Handle("/products/{slug}/feedback", auther.Wrap(addFeedbackHandler)).Methods("POST")

	getTokenHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenString, err := auther.GenerateToken()
		if err != nil {
			packageLogger.WithError(err).Warning("unable to sign JWT")
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(fmt.Sprintf("error signing token: %v", err)))
			return
		}

		w.Write([]byte(tokenString))
	})

	r.Handle("/get-token", getTokenHandler).Methods("GET")
}
