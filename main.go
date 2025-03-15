package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/basgys/goxml2json"
)

const (
	baseURL      = "https://www.mbsonline.gov.au/internet/mbsonline/publishing.nsf/Content/downloads"
	downloadPath = "downloads"
)

// Config holds the command-line arguments
type Config struct {
	execCmd      string
	webhookURL   string
	webhookHeaders string // JSON string of key-value pairs for headers
	force        bool
	sync         bool
}

// Field type definitions
type FieldType int

const (
	StringType FieldType = iota
	BooleanType
	DateType
	FloatType
)

// FieldInfo stores information about how to process each field
type FieldInfo struct {
	fieldType FieldType
	required  bool
}

// fieldDefinitions defines the type and requirements for each field
var fieldDefinitions = map[string]FieldInfo{
	// Required fields
	"ItemNum":     {StringType, true},  // Item number (required)
	"Description": {StringType, true},  // Description (required)

	// Boolean fields (Y/N)
	"NewItem":          {BooleanType, false},
	"ItemChange":       {BooleanType, false},
	"FeeChange":        {BooleanType, false},
	"BenefitChange":    {BooleanType, false},
	"AnaesChange":      {BooleanType, false},
	"EMSNChange":       {BooleanType, false},
	"DescriptorChange": {BooleanType, false},
	"Anaes":            {BooleanType, false},

	// Date fields (DD.MM.YYYY)
	"ItemStartDate":        {DateType, false},
	"ItemEndDate":          {DateType, false},
	"FeeStartDate":         {DateType, false},
	"BenefitStartDate":     {DateType, false},
	"DescriptionStartDate": {DateType, false},
	"EMSNStartDate":        {DateType, false},
	"EMSNEndDate":          {DateType, false},
	"QFEStartDate":         {DateType, false},
	"QFEEndDate":           {DateType, false},
	"DerivedFeeStartDate":  {DateType, false},
	"EMSNChangeDate":       {DateType, false},

	// Float fields (monetary amounts and percentages)
	"ScheduleFee":        {FloatType, false},
	"DerivedFee":         {FloatType, false},
	"Benefit75":          {FloatType, false},
	"Benefit85":          {FloatType, false},
	"Benefit100":         {FloatType, false},
	"EMSNPercentageCap":  {FloatType, false},
	"EMSNMaximumCap":     {FloatType, false},
	"EMSNFixedCapAmount": {FloatType, false},
	"EMSNCap":            {FloatType, false},
	"BasicUnits":         {FloatType, false},

	// String fields (everything else defaults to string)
	"Category":           {StringType, false},
	"Group":              {StringType, false},
	"SubGroup":           {StringType, false},
	"SubHeading":         {StringType, false},
	"ItemType":           {StringType, false},
	"SubItemNum":         {StringType, false},
	"BenefitType":        {StringType, false},
	"FeeType":            {StringType, false},
	"ProviderType":       {StringType, false},
	"EMSNDescription":    {StringType, false},
}

// convertValue converts a string value to its appropriate type based on the field definition
func convertValue(field string, value string) interface{} {
	// Get field info, default to string type if not defined
	fieldInfo, exists := fieldDefinitions[field]
	if !exists {
		return value
	}

	// Handle empty values
	if value == "" {
		switch fieldInfo.fieldType {
		case BooleanType:
			return false
		case DateType:
			return nil
		case FloatType:
			return 0.0
		default:
			return ""
		}
	}

	switch fieldInfo.fieldType {
	case BooleanType:
		return strings.ToUpper(value) == "Y"
	
	case DateType:
		// Parse date in DD.MM.YYYY format
		if t, err := time.Parse("02.01.2006", value); err == nil {
			return t.Format("2006-01-02") // Convert to ISO 8601 format
		}
		return nil

	case FloatType:
		// Try to parse as float
		if f, err := strconv.ParseFloat(value, 64); err == nil {
			return f
		}
		return 0.0

	default:
		return value
	}
}

// executeCommand runs the specified command with the JSON file path
func executeCommand(cmdTemplate string, jsonPath string, sync bool) error {
	// Replace {file} with the actual path
	cmd := strings.ReplaceAll(cmdTemplate, "{file}", jsonPath)
	
	// Split the command into program and arguments
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return fmt.Errorf("empty command")
	}

	// Create command
	command := exec.Command(parts[0], parts[1:]...)
	
	if sync {
		// Run synchronously
		log.Printf("Running command synchronously: %s", cmd)
		output, err := command.CombinedOutput()
		if err != nil {
			return fmt.Errorf("command failed: %v\nOutput: %s", err, string(output))
		}
		log.Printf("Command completed successfully: %s", cmd)
		return nil
	}
	
	// Run asynchronously (existing behavior)
	if err := command.Start(); err != nil {
		return fmt.Errorf("failed to start command: %v", err)
	}
	
	log.Printf("Started command in background: %s", cmd)

	go func() {
		err := command.Wait()
		if err != nil {
			log.Printf("Warning: Background command failed: %v", err)
		} else {
			log.Printf("Background command completed successfully: %s", cmd)
		}
	}()
	
	return nil
}

// sendWebhook sends the JSON file to the specified webhook URL
func sendWebhook(webhookURL string, webhookHeaders string, jsonPath string) error {
	// Read the JSON file
	jsonData, err := os.ReadFile(jsonPath)
	if err != nil {
		return fmt.Errorf("failed to read JSON file: %w", err)
	}

	// Create the request
	req, err := http.NewRequest("POST", webhookURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set default Content-Type header
	req.Header.Set("Content-Type", "application/json")

	// Parse and set custom headers if provided
	if webhookHeaders != "" {
		var headers map[string]string
		if err := json.Unmarshal([]byte(webhookHeaders), &headers); err != nil {
			return fmt.Errorf("failed to parse webhook headers: %w", err)
		}
		for key, value := range headers {
			req.Header.Set(key, value)
		}
	}

	// Send the request
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send webhook: %w", err)
	}
	defer resp.Body.Close()

	// Check response
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("webhook failed with status %d: %s", resp.StatusCode, string(body))
	}

	log.Printf("Webhook sent successfully to %s", webhookURL)
	return nil
}

// extractDateFromXMLLink extracts the date from an MBS XML filename
func extractDateFromXMLLink(xmlLink string) (string, error) {
	re := regexp.MustCompile(`MBS-XML-(\d{8})\.XML`)
	matches := re.FindStringSubmatch(xmlLink)
	if len(matches) < 2 {
		return "", fmt.Errorf("no date found in XML link: %s", xmlLink)
	}
	return matches[1], nil
}

// hasLatestVersion checks if we already have a JSON file for the given MBS date
func hasLatestVersion(mbsDate string) (bool, error) {
	// Read all files in the downloads directory
	files, err := os.ReadDir(downloadPath)
	if err != nil {
		return false, fmt.Errorf("failed to read downloads directory: %w", err)
	}

	// Look for any file containing the MBS date
	for _, file := range files {
		if strings.Contains(file.Name(), mbsDate) {
			return true, nil
		}
	}

	return false, nil
}

// validateJSON checks if the JSON structure is valid and consistent
func validateJSON(data map[string]interface{}) error {
	// Check if MBS_Items exists and is an array
	items, ok := data["MBS_Items"].([]interface{})
	if !ok {
		return fmt.Errorf("MBS_Items is not an array or is missing")
	}

	if len(items) == 0 {
		return fmt.Errorf("MBS_Items array is empty")
	}

	// First pass: collect all unique fields across all items
	allFields := make(map[string]bool)
	for _, item := range items {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		for field := range itemMap {
			allFields[field] = true
		}
	}

	// Convert allFields to a slice for logging
	var fieldNames []string
	for field := range allFields {
		fieldNames = append(fieldNames, field)
	}
	log.Printf("Found %d unique fields across all items: %v", len(fieldNames), fieldNames)

	// Second pass: validate and normalize items
	var validItems []interface{}
	for i, item := range items {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			log.Printf("Warning: Skipping item at index %d: not an object", i)
			continue
		}

		// Check required fields have non-empty values
		isValid := true
		for field, info := range fieldDefinitions {
			if !info.required {
				continue
			}
			value, exists := itemMap[field]
			if !exists {
				log.Printf("Warning: Skipping item at index %d: missing required field '%s'", i, field)
				isValid = false
				break
			}
			strValue, ok := value.(string)
			if !ok {
				log.Printf("Warning: Skipping item at index %d: field '%s' is not a string", i, field)
				isValid = false
				break
			}
			if strValue == "" {
				log.Printf("Warning: Skipping item at index %d: required field '%s' is empty", i, field)
				isValid = false
				break
			}
		}

		if !isValid {
			continue
		}

		// Create new item with converted types
		newItemMap := make(map[string]interface{})
		for field := range allFields {
			if value, exists := itemMap[field]; exists {
				// Convert value to string first
				strValue, ok := value.(string)
				if !ok {
					strValue = fmt.Sprintf("%v", value)
				}
				// Convert to appropriate type
				newItemMap[field] = convertValue(field, strValue)
			} else {
				// Handle missing fields with appropriate zero values
				newItemMap[field] = convertValue(field, "")
			}
		}

		// Add the normalized item to our valid items list
		validItems = append(validItems, newItemMap)
	}

	// Update the original data with normalized valid items
	data["MBS_Items"] = validItems

	log.Printf("JSON validation completed: %d valid items out of %d total items, %d fields per item", 
		len(validItems), len(items), len(allFields))
	return nil
}

func main() {
	// Parse command line flags
	config := Config{}
	flag.StringVar(&config.execCmd, "exec", "", "Command to execute when a new file is found. Use {file} as placeholder for the JSON path")
	flag.StringVar(&config.webhookURL, "webhook", "", "URL to POST the JSON file to when a new file is found")
	flag.StringVar(&config.webhookHeaders, "webhook-headers", "", "JSON string of headers to include in webhook request (e.g. '{\"Authorization\":\"Bearer token\",\"X-API-Key\":\"key\"}')")
	flag.BoolVar(&config.force, "force", false, "Force download even if the file already exists")
	flag.BoolVar(&config.sync, "sync", false, "Run the exec command synchronously instead of in the background")
	flag.Parse()

	// Enable debug logging
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// Create downloads directory if it doesn't exist
	if err := os.MkdirAll(downloadPath, 0755); err != nil {
		log.Fatal("Failed to create downloads directory:", err)
	}

	// Get the main downloads page
	doc, err := fetchPage(baseURL)
	if err != nil {
		log.Fatal("Failed to fetch downloads page:", err)
	}

	// Find the most recent MBS link
	latestLink := findLatestMBSLink(doc)
	if latestLink == "" {
		log.Fatal("Could not find latest MBS link")
	}
	log.Printf("Found latest link: %s", latestLink)

	// Get the download page
	downloadDoc, err := fetchPage(latestLink)
	if err != nil {
		log.Fatal("Failed to fetch download page:", err)
	}

	// Find the XML download link
	xmlLink := findXMLDownloadLink(downloadDoc)
	if xmlLink == "" {
		log.Fatal("Could not find XML download link")
	}
	log.Printf("Found XML link: %s", xmlLink)

	// Extract date from XML link
	mbsDate, err := extractDateFromXMLLink(xmlLink)
	if err != nil {
		log.Fatal("Failed to extract date from XML link:", err)
	}

	// Check if we already have this version
	hasVersion, err := hasLatestVersion(mbsDate)
	if err != nil {
		log.Fatal("Failed to check for existing version:", err)
	}

	if hasVersion && !config.force {
		log.Printf("Already have MBS version %s, skipping download (use -force to override)", mbsDate)
		return
	}

	// Download and process the XML file
	if err := downloadAndConvertXML(xmlLink); err != nil {
		log.Fatal("Failed to process XML:", err)
	}

	// Get the path of the newly created JSON file
	jsonPath := filepath.Join(downloadPath, fmt.Sprintf("mbs_%s.json", mbsDate))

	// Execute command if specified
	if config.execCmd != "" {
		if err := executeCommand(config.execCmd, jsonPath, config.sync); err != nil {
			log.Printf("Warning: Command execution failed: %v", err)
		}
	}

	// Send webhook if specified
	if config.webhookURL != "" {
		if err := sendWebhook(config.webhookURL, config.webhookHeaders, jsonPath); err != nil {
			log.Printf("Warning: Webhook failed: %v", err)
		}
	}

	fmt.Println("Successfully downloaded and converted MBS data!")
}

func fetchPage(url string) (*goquery.Document, error) {
	log.Printf("Fetching page: %s", url)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP request failed with status: %d", resp.StatusCode)
	}

	return goquery.NewDocumentFromReader(resp.Body)
}

func findLatestMBSLink(doc *goquery.Document) string {
	var latestLink string
	var latestDate time.Time

	// Regular expression to match month year format
	dateRegex := regexp.MustCompile(`(January|February|March|April|May|June|July|August|September|October|November|December)\s+\d{4}`)

	// Look for links containing dates
	doc.Find("a").Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if !exists {
			return
		}

		text := s.Text()
		log.Printf("Examining link: text='%s', href='%s'", text, href)

		// Look for text containing dates
		if match := dateRegex.FindString(text); match != "" {
			date, err := time.Parse("January 2006", match)
			if err == nil && (latestDate.IsZero() || date.After(latestDate)) {
				latestDate = date
				latestLink = href
				log.Printf("Found potential latest link: %s (date: %s)", href, date)
			}
		}
	})

	// If the link is relative, make it absolute
	if latestLink != "" && !strings.HasPrefix(latestLink, "http") {
		if strings.HasPrefix(latestLink, "/") {
			latestLink = "https://www.mbsonline.gov.au" + latestLink
		} else {
			latestLink = "https://www.mbsonline.gov.au/internet/mbsonline/publishing.nsf/Content/" + latestLink
		}
	}

	return latestLink
}

func findXMLDownloadLink(doc *goquery.Document) string {
	var xmlLink string
	// Regular expression to match MBS XML files
	mbsXMLRegex := regexp.MustCompile(`(?i)MBS-XML-\d{8}\.XML$`)

	doc.Find("a").Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if !exists {
			return
		}

		text := strings.ToLower(s.Text())
		log.Printf("Examining download link: text='%s', href='%s'", text, href)
		
		// Look for links that match the MBS XML pattern
		if mbsXMLRegex.MatchString(href) || mbsXMLRegex.MatchString(text) || strings.Contains(text, "mbs-xml") {
			// If the link contains a File directory, it's likely the correct one
			if strings.Contains(href, "/$File/") {
				xmlLink = href
				log.Printf("Found MBS XML link: %s", href)
			}
		}
	})

	// If the link is relative, make it absolute
	if xmlLink != "" && !strings.HasPrefix(xmlLink, "http") {
		if strings.HasPrefix(xmlLink, "/") {
			xmlLink = "https://www.mbsonline.gov.au" + xmlLink
		} else {
			xmlLink = "https://www.mbsonline.gov.au/internet/mbsonline/publishing.nsf/Content/" + xmlLink
		}
	}

	return xmlLink
}

func downloadAndConvertXML(url string) error {
	log.Printf("Downloading XML from: %s", url)
	
	// Extract date from URL for the filename
	mbsDate, err := extractDateFromXMLLink(url)
	if err != nil {
		return fmt.Errorf("failed to extract date from URL: %w", err)
	}

	// Download XML file
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to download XML: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("XML download failed with status: %d", resp.StatusCode)
	}

	// Read the XML content
	xmlData, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read XML data: %w", err)
	}

	log.Printf("Successfully downloaded XML (%d bytes)", len(xmlData))

	// Convert XML to JSON
	jsonData, err := xml2json.Convert(bytes.NewReader(xmlData))
	if err != nil {
		return fmt.Errorf("failed to convert XML to JSON: %w", err)
	}

	// Parse the JSON to modify its structure
	var rawJSON map[string]interface{}
	if err := json.Unmarshal(jsonData.Bytes(), &rawJSON); err != nil {
		return fmt.Errorf("failed to parse JSON: %w", err)
	}

	// Extract and rename the data
	mbsXML, ok := rawJSON["MBS_XML"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("unexpected JSON structure: missing MBS_XML object")
	}

	data, ok := mbsXML["Data"]
	if !ok {
		return fmt.Errorf("unexpected JSON structure: missing Data object")
	}

	// Create new structure with renamed node
	newJSON := map[string]interface{}{
		"MBS_Items": data,
	}

	// Validate the JSON structure
	if err := validateJSON(newJSON); err != nil {
		return fmt.Errorf("JSON validation failed: %w", err)
	}

	// Pretty print the modified JSON
	var prettyJSON bytes.Buffer
	encoder := json.NewEncoder(&prettyJSON)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(newJSON); err != nil {
		return fmt.Errorf("failed to format JSON: %w", err)
	}

	// Generate filename with MBS date
	filename := filepath.Join(downloadPath, fmt.Sprintf("mbs_%s.json", mbsDate))

	// Save the JSON to file
	if err := os.WriteFile(filename, prettyJSON.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to save JSON file: %w", err)
	}

	fmt.Printf("Saved JSON data to: %s\n", filename)
	return nil
} 