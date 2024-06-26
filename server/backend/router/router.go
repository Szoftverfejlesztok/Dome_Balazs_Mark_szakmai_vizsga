package router

import (
	"backend/db"
	"backend/misc"
	"backend/types"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{}

// AddRecordHandler handler for /addRecord POST request
func AddRecordHandler(w http.ResponseWriter, r *http.Request) {
	slog.Info("Got AddRecord POST request")

	event := &types.Device{}
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&event); err != nil {
		http.Error(w, "Error unmarshalling request body", http.StatusBadRequest)
		return
	}
	slog.Info("Request body", slog.String("device", event.Device), slog.Bool("state", event.State))
	if _, err := db.IsDeviceExist(event.Device); err != nil {
		http.Error(w, "Error this device does not exist", http.StatusBadRequest)
	}

	// Create and add a record to the database
	record := types.Device{
		Id:     0,
		Device: event.Device,
		Date:   "",
		State:  event.State,
	}
	record, err := db.AddRecord(record)
	if err != nil {
		slog.Error("Error adding record to the database", slog.Any("error", err),
			slog.Any("record", record))
		http.Error(w, "Error adding record to the database", http.StatusInternalServerError)
		return
	}

	resp, err := json.Marshal(record)
	if err != nil {
		slog.Error("Error marshalling response")
	}

	if _, err = w.Write(resp); err != nil {
		slog.Error("Could not serve request for AddRecord")
	}
}

// GetLastByDeviceHandler handler for /getLastByDevice/<device> GET requests
func GetLastByDeviceHandler(w http.ResponseWriter, r *http.Request) {
	device := chi.URLParam(r, "device")
	slog.Info("Got GetLastByDevice GET request", slog.String("device", device))
	if _, err := db.IsDeviceExist(device); err != nil {
		http.Error(w, "Error this device does not exist", http.StatusBadRequest)
	}

	record, err := db.GetLastByDevice(device)
	if err != nil {
		slog.Error("Error getting record from the database", slog.Any("error", err),
			slog.String("device", device))
		http.Error(w, "Error getting record from the database", http.StatusInternalServerError)
	}

	resp, err := json.Marshal(record)
	if err != nil {
		slog.Error("Error marshalling response")
	}

	if _, err = w.Write(resp); err != nil {
		slog.Error("Could not serve request for GetLastByDevice")
	}
}

// GetDevices
func GetDevices(w http.ResponseWriter, r *http.Request) {
	slog.Info("Got GetDevices request")

	devices, err := db.GetDistinctDevice()
	if err != nil {
		slog.Error("Error getting devices", slog.Any("error", err))
		http.Error(w, "Error getting record from the database", http.StatusInternalServerError)
	}
	var resp []byte
	resp, err = json.Marshal(devices)
	if err != nil {
		slog.Error("Error marshalling response")
	}

	if _, err = w.Write(resp); err != nil {
		slog.Error("Could not serve request for Getdevices")
	}
}

// GetDevicesUptime
func GetDevicesUptime(w http.ResponseWriter, r *http.Request) {
	slog.Info("Got GetDevicesUptime request")

	devices, err := db.GetDevicesUptime()
	if err != nil {
		slog.Error("Error getting devices uptime", slog.Any("error", err))
		http.Error(w, "Error getting record from the database", http.StatusInternalServerError)
	}
	var resp []byte
	resp, err = json.Marshal(devices)
	if err != nil {
		slog.Error("Error marshalling response")
	}

	if _, err = w.Write(resp); err != nil {
		slog.Error("Could not serve request for GetdevicesUptime")
	}
}

// HealthCheckHandler handler for /hc GET requests
func HealthCheckHandler(w http.ResponseWriter, r *http.Request) {
	slog.Info("Got HealthCheck GET request")

	var resp = "OK"
	if err := db.HealthCheck(); err != nil {
		resp = "NOT_OK"
		slog.Error("Could not connect to the database")
	}
	if _, err := w.Write([]byte(resp + "\n")); err != nil {
		slog.Error("Could not serve request for HealthCheck", slog.Any("error", err))
	}
}

// FileServer serve a static file server based on the given folder
func FileServer(r chi.Router, path string, root http.FileSystem) {
	if strings.ContainsAny(path, "{}*") {
		panic("FileServer does not permit any URL parameters.")
	}

	if path != "/" && path[len(path)-1] != '/' {
		r.Get(path, http.RedirectHandler(path+"/", 301).ServeHTTP)
		path += "/"
	}
	path += "*"

	r.Get(path, func(w http.ResponseWriter, r *http.Request) {
		rctx := chi.RouteContext(r.Context())
		pathPrefix := strings.TrimSuffix(rctx.RoutePattern(), "/*")
		fs := http.StripPrefix(pathPrefix, http.FileServer(root))
		fs.ServeHTTP(w, r)
	})
}

// HandleClients handler for the /smart-home WS connections
func HandleClient(w http.ResponseWriter, r *http.Request) {
	slog.Info("Controller tries to connect")
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("Error connection to upgrade", slog.Any("error", err))
		return
	}
	defer conn.Close()

	slog.Info("Controller connected")

	// SENDING DATA
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			if strings.Contains(err.Error(), "connection reset by peer") {
				slog.Warn("Controller disconnected")
				return
			} else if strings.Contains(err.Error(), "connection timed out") {
				slog.Warn("Controloller timeouted")
				return
			} else {
				slog.Error("Error read message", slog.Any("error", err))
				return
			}
		}
		if string(msg) != "OK\n" {
			slog.Info("Received message from client", slog.String("client", conn.RemoteAddr().String()),
				slog.String("message", string(msg)))
		}

		states, err := db.GetStates()
		if err != nil {
			slog.Error("Error getting device states from database", slog.Any("error", err))
			return
		}

		// Send the XORed data
		if err := conn.WriteMessage(websocket.TextMessage, []byte(misc.XorData(states))); err != nil {
			slog.Error("Error write message", slog.Any("error", err))
			return
		}
	}
}
