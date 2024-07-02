package main

import (
	"database/sql"
	"ecommerce/middlewares"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
	"golang.org/x/crypto/bcrypt"
)

var (
	db    *sql.DB
	tpl   *template.Template
	store = sessions.NewCookieStore([]byte("your-secret-key"))
)

type User struct {
	ID       int
	Username string
	Password string
	Role     string
}

type Product struct {
	ID          int
	Name        string
	Description string
	Price       float64
	ImagePath   string // Tambahkan field untuk menyimpan path gambar dari database
}

func init() {
	var err error
	db, err = sql.Open("mysql", "root:admin@tcp(127.0.0.1:3306)/ecommerce")
	if err != nil {
		log.Fatal(err)
	}

	tpl = template.Must(template.ParseGlob("templates/*.html"))
}

func main() {
	r := mux.NewRouter()
	r.HandleFunc("/", homeHandler)
	r.HandleFunc("/index.html", homeHandler)
	r.HandleFunc("/login", loginHandler)
	r.HandleFunc("/logout", logoutHandler)
	r.HandleFunc("/register", registerHandler)
	r.HandleFunc("/product", productHandler)
	r.HandleFunc("/add_product", addProductHandler).Methods("GET", "POST")
	r.Handle("/my_product", middlewares.IsAuthenticatedMiddleware(http.HandlerFunc(myProductHandler)))
	r.HandleFunc("/checkout", checkoutHandler).Methods("GET", "POST")
	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	r.PathPrefix("/templates/").Handler(http.StripPrefix("/templates/", http.FileServer(http.Dir("templates"))))

	log.Println("Server started on :8080")
	log.Fatal(http.ListenAndServe(":8080", r))
}

func homeHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query("SELECT id, name, description, price FROM products")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var products []Product
	for rows.Next() {
		var p Product
		err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.Price)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		products = append(products, p)
	}

	isAuth := isAuthenticated(r)

	data := struct {
		Products        []Product
		IsAuthenticated bool
	}{
		Products:        products,
		IsAuthenticated: isAuth,
	}

	err = tpl.ExecuteTemplate(w, "index.html", data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		username := r.FormValue("username")
		password := r.FormValue("password")

		var user User
		err := db.QueryRow("SELECT id, username, password, role FROM users WHERE username = ?", username).Scan(&user.ID, &user.Username, &user.Password, &user.Role)
		if err != nil {
			http.Error(w, "Invalid username or password", http.StatusUnauthorized)
			return
		}

		err = bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password))
		if err != nil {
			http.Error(w, "Invalid username or password", http.StatusUnauthorized)
			return
		}

		session, _ := store.Get(r, "session-name")
		session.Values["authenticated"] = true
		session.Save(r, w)

		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	tpl.ExecuteTemplate(w, "login.html", nil)
}

func logoutHandler(w http.ResponseWriter, r *http.Request) {
	session, err := store.Get(r, "session-name")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	session.Values["authenticated"] = false
	session.Save(r, w)

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func registerHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		username := r.FormValue("username")
		password := r.FormValue("password")
		role := r.FormValue("role")

		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		_, err = db.Exec("INSERT INTO users (username, password, role) VALUES (?, ?, ?)", username, hashedPassword, role)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	tpl.ExecuteTemplate(w, "register.html", nil)
}

func productHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// Menangani permintaan GET
		rows, err := db.Query("SELECT id, name, description, price, image_path FROM products")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var products []Product
		for rows.Next() {
			var p Product
			err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.Price, &p.ImagePath)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			products = append(products, p)
		}

		isAuth := isAuthenticated(r)

		data := struct {
			Products        []Product
			IsAuthenticated bool
		}{
			Products:        products,
			IsAuthenticated: isAuth,
		}

		err = tpl.ExecuteTemplate(w, "product.html", data)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}

	case http.MethodPost:
		// Menangani permintaan POST
		err := r.ParseForm()
		if err != nil {
			http.Error(w, "Gagal memproses form", http.StatusBadRequest)
			return
		}

		imageID := r.FormValue("image_id")
		if imageID == "" {
			http.Error(w, "ID gambar tidak ditemukan", http.StatusBadRequest)
			return
		}

		var imagePath string
		err = db.QueryRow("SELECT image_path FROM products WHERE id = ?", imageID).Scan(&imagePath)
		if err != nil {
			http.Error(w, "Gambar tidak ditemukan", http.StatusNotFound)
			return
		}

		http.ServeFile(w, r, imagePath)

	default:
		http.Error(w, "Metode tidak diizinkan", http.StatusMethodNotAllowed)
	}
}

func myProductHandler(w http.ResponseWriter, r *http.Request) {
	if !isAuthenticated(r) {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	switch r.Method {
	case http.MethodGet:
		rows, err := db.Query("SELECT id, name, description, price, image_path FROM products")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var products []Product
		for rows.Next() {
			var p Product
			err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.Price, &p.ImagePath)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			products = append(products, p)
		}

		data := struct {
			Products []Product
		}{
			Products: products,
		}

		err = tpl.ExecuteTemplate(w, "my_product.html", data)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

	case http.MethodPost:
		r.ParseForm()
		action := r.FormValue("action")
		switch action {

		case "delete":
			productID := r.FormValue("product_id")
			id, err := strconv.Atoi(productID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			_, err = db.Exec("DELETE FROM products WHERE id = ?", id)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			http.Redirect(w, r, "/my_product", http.StatusSeeOther)

		case "edit":
			productID := r.FormValue("product_id")
			id, err := strconv.Atoi(productID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			name := r.FormValue("name")
			description := r.FormValue("description")
			priceStr := r.FormValue("price")
			price, err := strconv.ParseFloat(priceStr, 64)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			_, err = db.Exec("UPDATE products SET name=?, description=?, price=? WHERE id=?", name, description, price, id)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			http.Redirect(w, r, "/my_product", http.StatusSeeOther)

		case "view_image":
			imageID := r.FormValue("image_id")
			if imageID == "" {
				http.Error(w, "ID gambar tidak ditemukan", http.StatusBadRequest)
				return
			}

			var imagePath string
			err := db.QueryRow("SELECT image_path FROM products WHERE id = ?", imageID).Scan(&imagePath)
			if err != nil {
				http.Error(w, "Gambar tidak ditemukan", http.StatusNotFound)
				return
			}

			http.ServeFile(w, r, imagePath)

		default:
			http.Error(w, "Invalid action", http.StatusBadRequest)
			return
		}
	default:
		http.Error(w, "Metode tidak diizinkan", http.StatusMethodNotAllowed)
	}
}

func addProductHandler(w http.ResponseWriter, r *http.Request) {
	if !isAuthenticated(r) {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	switch r.Method {
	case http.MethodGet:
		err := tpl.ExecuteTemplate(w, "add_product.html", nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

	case http.MethodPost:
		r.ParseMultipartForm(10 << 20) // 10 MB max file size
		action := r.FormValue("action")
		switch action {
		case "add":
			name := r.FormValue("name")
			description := r.FormValue("description")
			priceStr := r.FormValue("price")

			price, err := strconv.ParseFloat(priceStr, 64)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			// Simpan gambar ke server
			file, handler, err := r.FormFile("image")
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			defer file.Close()

			// Tentukan direktori penyimpanan gambar
			imageDir := "ecommerce/static/images"
			os.MkdirAll(imageDir, os.ModePerm)

			// Simpan gambar dengan nama unik
			imageFileName := uuid.New().String() + filepath.Ext(handler.Filename)
			imagePath := filepath.Join(imageDir, imageFileName)
			f, err := os.OpenFile(imagePath, os.O_WRONLY|os.O_CREATE, 0666)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			defer f.Close()
			io.Copy(f, file)

			// Simpan path gambar ke database
			_, err = db.Exec("INSERT INTO products (name, description, price, image_path) VALUES (?, ?, ?, ?)", name, description, price, imagePath)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			http.Redirect(w, r, "/my_product", http.StatusSeeOther)

		default:
			http.Error(w, "Aksi tidak valid", http.StatusBadRequest)
			return
		}

	default:
		http.Error(w, "Metode tidak diizinkan", http.StatusMethodNotAllowed)
		return
	}
}

func checkoutHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if !isAuthenticated(r) {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		// Tampilkan halaman checkout
		err := tpl.ExecuteTemplate(w, "checkout.html", nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}

	case http.MethodPost:
		if !isAuthenticated(r) {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		// Ambil data dari formulir
		cardNumber := r.FormValue("cardNumber")
		expiryDate := r.FormValue("expiryDate")
		cvv := r.FormValue("cvv")

		// Validasi input (jika diperlukan)
		if cardNumber == "" || expiryDate == "" || cvv == "" {
			http.Error(w, "Payment form is incomplete!", http.StatusBadRequest)
			return
		}

		// Simpan data pembayaran ke database
		paymentDate := time.Now().Format("2006-01-02 15:04:05")
		_, err := db.Exec("INSERT INTO payments (card_number, expiry_date, cvv, payment_date) VALUES (?, ?, ?, ?)", cardNumber, expiryDate, cvv, paymentDate)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Tampilkan halaman checkout kembali dengan pesan sukses
		successMessage := "Payment successfull!"
		data := struct {
			SuccessMessage string
		}{
			SuccessMessage: successMessage,
		}
		err = tpl.ExecuteTemplate(w, "checkout.html", data)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

func isAuthenticated(r *http.Request) bool {
	session, err := store.Get(r, "session-name")
	if err != nil {
		return false
	}

	if auth, ok := session.Values["authenticated"].(bool); !ok || !auth {
		return false
	}

	return true
}
