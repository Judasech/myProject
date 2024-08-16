package main

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/gorilla/mux"
)

func main() {
	router := mux.NewRouter()
	router.HandleFunc("/reserve", ReserveProducts).Methods("POST")
	router.HandleFunc("/release", ReleaseReservedProducts).Methods("POST")
	router.HandleFunc("/quantity", GetRemainingQuantity).Methods("GET")

	log.Fatal(http.ListenAndServe(":8080", router))
}
func GetRemainingQuantity(w http.ResponseWriter, r *http.Request) {
	warehouseID := r.URL.Query().Get("warehouse_id")

	var quantity int
	query := `SELECT SUM(quantity)
              FROM product p
              JOIN reservation r ON p.unique_code = r.product_unique_code
              WHERE r.warehouse_id = $1`

	err := db.Get(&quantity, query, warehouseID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response := map[string]int{"remaining_quantity": quantity}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
func ReleaseReservedProducts(w http.ResponseWriter, r *http.Request) {
	var request struct {
		WarehouseID  int      `json:"warehouse_id"`
		ProductCodes []string `json:"product_codes"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	tx, err := db.Beginx()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	for _, code := range request.ProductCodes {
		var reservedQuantity int
		err = tx.Get(&reservedQuantity, "SELECT reserved_quantity FROM reservation WHERE product_unique_code = $1 AND warehouse_id = $2 FOR UPDATE", code, request.WarehouseID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if reservedQuantity <= 0 {
			http.Error(w, "No reserved quantity to release", http.StatusConflict)
			return
		}

		_, err = tx.Exec("UPDATE reservation SET reserved_quantity = reserved_quantity - 1 WHERE product_unique_code = $1 AND warehouse_id = $2", code, request.WarehouseID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		_, err = tx.Exec("UPDATE product SET quantity = quantity + 1 WHERE unique_code = $1", code)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	err = tx.Commit()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
func ReserveProducts(w http.ResponseWriter, r *http.Request) {
	var request struct {
		WarehouseID  int      `json:"warehouse_id"`
		ProductCodes []string `json:"product_codes"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	tx, err := db.Beginx()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	for _, code := range request.ProductCodes {
		var quantity int
		err = tx.Get(&quantity, "SELECT quantity FROM product WHERE unique_code = $1 FOR UPDATE", code)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if quantity <= 0 {
			http.Error(w, "Insufficient quantity", http.StatusConflict)
			return
		}

		_, err = tx.Exec(`
            INSERT INTO reservation (product_unique_code, warehouse_id, reserved_quantity)
            VALUES ($1, $2, 1)
            ON CONFLICT (product_unique_code, warehouse_id)
            DO UPDATE SET reserved_quantity = reservation.reserved_quantity + 1`, code, request.WarehouseID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		_, err = tx.Exec("UPDATE product SET quantity = quantity - 1 WHERE unique_code = $1", code)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	err = tx.Commit()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
