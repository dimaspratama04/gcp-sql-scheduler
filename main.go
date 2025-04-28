package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/joho/godotenv"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
	"google.golang.org/api/sqladmin/v1"
)

type SQLInstancesData struct {
	Name            string `json:"name"`
	DatabaseVersion string `json:"database_version"`
	Region          string `json:"region"`
	State           string `json:"state"`
	Tier            string `json:"tier"`
}

type TemplateSuccessResponse struct {
	StatusCode int              `json:"status_code"`
	StatusText string           `json:"status_text"`
	Message    string           `json:"message"`
	Timestamp  string           `json:"timestamp"`
	Data       SQLInstancesData `json:"data"`
}

type TemplateErrorResponse struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

type ErrorJSON struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

var (
	projectID  string
	instanceID string
	port       string
)

func init() {
	if os.Getenv("ENV") == "local" {
		err := godotenv.Load(".env")
		if err != nil {
			log.Print("Error loading .env file")
		}
	}

	projectID = os.Getenv("PROJECT_ID")
	instanceID = os.Getenv("INSTANCE_ID")
	port = os.Getenv("PORT")

	if port == "" {
		port = "80"
	}
}

func main() {
	http.HandleFunc("/stop", stopInstancesHandler)
	http.HandleFunc("/start", startInstanceHandler)
	http.HandleFunc("/check", checkInstancesHandler)

	fmt.Println("Server running at http://localhost:" + port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
	}
}

func startInstanceHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed.", "")
		return
	}

	ctx := context.Background()
	sqlService, err := sqladmin.NewService(ctx, option.WithCredentialsFile("service_account.json"))
	if err != nil {
		writeErrorResponse(w, http.StatusUnauthorized, "Service Account not found.", err)
		return
	}

	_, err = checkStatusInstances(projectID, instanceID)
	if err != nil {
		writeErrorResponse(w, http.StatusInternalServerError, "Instances not found.", err.Error())
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeErrorResponse(w, http.StatusBadRequest, "Failed to read request body.", err)
		return
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		writeErrorResponse(w, http.StatusBadRequest, "Invalid JSON format.", err)
		return
	}

	activationPolicy, ok := payload["ActivationPolicy"].(string)
	if !ok || (activationPolicy != "ALWAYS" && activationPolicy != "NEVER") {
		writeErrorResponse(w, http.StatusBadRequest, "Invalid value for ActivationPolicy. Must be 'ALWAYS' or 'NEVER'.", "")
		return
	}

	payloadDoStartInstances := &sqladmin.DatabaseInstance{
		Settings: &sqladmin.Settings{
			ActivationPolicy: activationPolicy, // START
		},
	}

	doStartInstances, err := sqlService.Instances.Patch(projectID, instanceID, payloadDoStartInstances).Do()
	if err != nil {
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to start instance.", err)
		return
	}

	writeSuccessResponse(w, http.StatusOK, "Instance successfully started. Check console for details.", *doStartInstances)
}

func stopInstancesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed.", "")
		return
	}

	ctx := context.Background()
	sqlService, err := sqladmin.NewService(ctx, option.WithCredentialsFile("service_account.json"))
	if err != nil {
		writeErrorResponse(w, http.StatusUnauthorized, "Service Account not found.", err)
		return
	}

	status, err := checkStatusInstances(projectID, instanceID)
	if err != nil {
		writeErrorResponse(w, http.StatusInternalServerError, "Instances not found.", err.Error())
		return
	}

	if status.State != "RUNNABLE" {
		writeErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("Instance currently in %s state.", status.State), "")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeErrorResponse(w, http.StatusBadRequest, "Failed to read request body.", err)
		return
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		writeErrorResponse(w, http.StatusBadRequest, "Invalid JSON format.", err)
		return
	}

	activationPolicy, ok := payload["ActivationPolicy"].(string)
	if !ok || (activationPolicy != "ALWAYS" && activationPolicy != "NEVER") {
		writeErrorResponse(w, http.StatusBadRequest, "Invalid value for ActivationPolicy. Must be 'ALWAYS' or 'NEVER'.", "")
		return
	}

	payloadDoStopInstances := &sqladmin.DatabaseInstance{
		Settings: &sqladmin.Settings{
			ActivationPolicy: activationPolicy, // STOP

		},
	}

	doStopInstances, err := sqlService.Instances.Patch(projectID, instanceID, payloadDoStopInstances).Do()
	if err != nil {
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to stop instance.", err)
		return
	}

	writeSuccessResponse(w, http.StatusOK, "Instance successfully stopped. Check console for details.", *doStopInstances)
}

func checkInstancesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed.", "")
		return
	}
	ctx := context.Background()
	_, err := sqladmin.NewService(ctx, option.WithCredentialsFile("service_account.json"))
	if err != nil {
		writeErrorResponse(w, http.StatusUnauthorized, "Service Account not found.", err)
		return
	}

	instance, err := checkStatusInstances(projectID, instanceID)
	if err != nil {
		writeErrorResponse(w, http.StatusInternalServerError, "Instances not found.", err.Error())
		return
	}

	responseData := &SQLInstancesData{
		Name:            instance.Name,
		DatabaseVersion: instance.DatabaseVersion,
		Region:          instance.Region,
		State:           instance.State,
		Tier:            instance.Tier,
	}

	writeSuccessResponse(w, http.StatusOK, "Successfully fetch instances detail.", responseData)
}

func writeSuccessResponse(w http.ResponseWriter, statusCode int, message string, data interface{}) {
	response := map[string]interface{}{
		"data":        data,
		"status_code": statusCode,
		"status_text": http.StatusText(statusCode),
		"message":     message,
		"timestamp":   time.Now().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(response)
}

func writeErrorResponse(w http.ResponseWriter, statusCode int, message string, err interface{}) {
	var errorType string
	var errorDescription string

	switch e := err.(type) {
	case *googleapi.Error:
		errorType = fmt.Sprintf("googleapi_%d", e.Code)
		errorDescription = e.Message
	case error:
		errorType = "internal_error"
		errorDescription = e.Error()
	case string:
		errorType = "internal_error"
		errorDescription = e
	default:
		errorType = "unknown_error"
		errorDescription = fmt.Sprintf("%v", e)
	}

	response := map[string]interface{}{
		"status_code":       statusCode,
		"status_text":       http.StatusText(statusCode),
		"message":           message,
		"timestamp":         time.Now().Format(time.RFC3339),
		"error_type":        errorType,
		"error_description": errorDescription,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(response)
}

func checkStatusInstances(projectID string, instanceID string) (*SQLInstancesData, error) {
	ctx := context.Background()
	sqlService, err := sqladmin.NewService(ctx, option.WithCredentialsFile("service_account.json"))
	if err != nil {
		return nil, fmt.Errorf("failed to find Service Account: %w", err)
	}

	instance, err := sqlService.Instances.Get(projectID, instanceID).Do()
	if err != nil {
		return nil, fmt.Errorf("failed to get instance details, instances not found.: %w", err)
	}

	responseData := &SQLInstancesData{
		Name:            instance.Name,
		DatabaseVersion: instance.DatabaseVersion,
		Region:          instance.Region,
		State:           instance.State,
		Tier:            instance.Settings.Tier,
	}

	return responseData, nil
}
