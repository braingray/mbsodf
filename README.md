# MBS Online Data Fetcher

This Go application fetches the latest Medicare Benefits Schedule (MBS) data from the Australian government website and converts it from XML to JSON format.

## Features

- Automatically finds the most recent MBS data file
- Checks if the latest version is already downloaded to avoid redundant processing
- Downloads the XML file if a new version is available
- Converts the XML to nicely formatted JSON with proper data types
- Restructures the JSON to remove unnecessary nesting
- Saves the result in a `downloads` directory with the MBS version date
- Can execute a custom command asynchronously when new data is downloaded
- Can send the JSON data to a webhook URL
- Supports forced download to override existing files

## Prerequisites

- Go 1.16 or later

## Installation

1. Clone this repository
2. Install dependencies:
```bash
go mod download
```

## Usage

Basic usage:
```bash
go run main.go
```

Force download even if file exists:
```bash
go run main.go -force
```

With command execution when new data is found:
```bash
go run main.go -exec "notepad.exe {file}"
```
The `{file}` placeholder will be replaced with the path to the new JSON file.

With webhook POST when new data is found:
```bash
go run main.go -webhook "https://your-server.com/webhook"
```

You can combine multiple options:
```bash
# Execute command and send webhook
go run main.go -exec "notepad.exe {file}" -webhook "https://your-server.com/webhook"

# Force download and execute command
go run main.go -force -exec "notepad.exe {file}"
```

The program will:
1. Create a `downloads` directory if it doesn't exist
2. Find the most recent MBS data file from the government website
3. Check if this version has already been downloaded
4. If it's a new version (or if -force is used):
   - Download the XML file
   - Convert it to JSON with proper data types
   - Remove unnecessary nesting (MBS_XML parent node)
   - Rename the Data node to MBS_Items
   - Save the JSON file in the `downloads` directory
   - Launch the specified command in the background (if -exec is provided)
   - Send the JSON to the webhook URL (if -webhook is provided)
5. If the version already exists and -force is not used, skip all processing

The output file will be named in the format: `mbs_YYYYMMDD.json` where YYYYMMDD is the MBS version date.

## JSON Structure

The output JSON has the following structure:

```json
{
  "MBS_Items": [
    {
      // Required fields
      "ItemNum": "string",      // Item number (required)
      "Description": "string",  // Description (required)

      // Boolean fields (true/false)
      "NewItem": boolean,          // Y/N in XML
      "ItemChange": boolean,       // Y/N in XML
      "FeeChange": boolean,        // Y/N in XML
      "BenefitChange": boolean,    // Y/N in XML
      "AnaesChange": boolean,      // Y/N in XML
      "EMSNChange": boolean,       // Y/N in XML
      "DescriptorChange": boolean, // Y/N in XML
      "Anaes": boolean,            // Y/N in XML

      // Date fields (ISO 8601: YYYY-MM-DD)
      "ItemStartDate": string,        // DD.MM.YYYY in XML
      "ItemEndDate": string,          // DD.MM.YYYY in XML
      "FeeStartDate": string,         // DD.MM.YYYY in XML
      "BenefitStartDate": string,     // DD.MM.YYYY in XML
      "DescriptionStartDate": string, // DD.MM.YYYY in XML
      "EMSNStartDate": string,        // DD.MM.YYYY in XML
      "EMSNEndDate": string,          // DD.MM.YYYY in XML
      "QFEStartDate": string,         // DD.MM.YYYY in XML
      "QFEEndDate": string,           // DD.MM.YYYY in XML
      "DerivedFeeStartDate": string,  // DD.MM.YYYY in XML
      "EMSNChangeDate": string,       // DD.MM.YYYY in XML

      // Float fields (numeric values)
      "ScheduleFee": number,        // Fee amount
      "DerivedFee": number,         // Derived fee amount
      "Benefit75": number,          // 75% benefit
      "Benefit85": number,          // 85% benefit
      "Benefit100": number,         // 100% benefit
      "EMSNPercentageCap": number,  // EMSN percentage cap
      "EMSNMaximumCap": number,     // EMSN maximum cap
      "EMSNFixedCapAmount": number, // EMSN fixed cap amount
      "EMSNCap": number,            // EMSN cap
      "BasicUnits": number,         // Basic units

      // String fields
      "Category": string,         // Category identifier
      "Group": string,           // Group identifier
      "SubGroup": string,        // Sub-group identifier
      "SubHeading": string,      // Sub-heading text
      "ItemType": string,        // Item type
      "SubItemNum": string,      // Sub-item number
      "BenefitType": string,     // Benefit type
      "FeeType": string,         // Fee type
      "ProviderType": string,    // Provider type
      "EMSNDescription": string  // EMSN description
    }
  ]
}
```

### Field Type Handling

- **Boolean fields**: Convert "Y" to `true`, "N" or empty to `false`
- **Date fields**: Convert from DD.MM.YYYY to ISO 8601 (YYYY-MM-DD) format
- **Float fields**: Parse numeric values as 64-bit floating point
- **String fields**: Preserve as strings
- **Missing fields**: Added with appropriate zero values:
  - Boolean: `false`
  - Date: `null`
  - Float: `0.0`
  - String: `""`

### Command Execution (-exec, -sync)

The -exec flag allows you to specify a command to run when new data is downloaded. The command can include the special placeholder `{file}` which will be replaced with the path to the new JSON file.

By default, commands are executed asynchronously (in the background), meaning:
- The program won't wait for the command to complete
- You'll see a "Started command in background" message immediately
- Command success/failure will be logged separately
- The program will continue running even if the command is still executing

You can use the -sync flag to run commands synchronously instead:
- The program will wait for the command to complete before continuing
- Command output will be captured and displayed if there's an error
- Success/failure will be reported immediately
- Useful when subsequent operations depend on the command's completion

Examples:
```bash
# Asynchronous execution (default)
go run main.go -exec "notepad.exe {file}"

# Synchronous execution
go run main.go -exec "python process_mbs.py {file}" -sync

# Synchronous execution with force download
go run main.go -force -exec "python process_mbs.py {file}" -sync

# Asynchronous execution with webhook
go run main.go -exec "notepad.exe {file}" -webhook "https://api.example.com/webhook"
```

Common use cases for -sync:
- Running data processing scripts that must complete before sending to webhook
- Ensuring file operations complete before program exit
- Sequential processing where order matters
- Debugging command execution issues

### Webhook Integration (-webhook, -webhook-headers)

The -webhook flag allows you to specify a URL where the JSON data will be sent via HTTP POST when new data is downloaded. You can also specify custom headers using the -webhook-headers flag.

- The request will have Content-Type: application/json by default
- Custom headers can be provided as a JSON string
- The entire JSON file will be sent in the request body
- The webhook must return a 2xx status code to be considered successful
- The request will timeout after 30 seconds

Examples:
```bash
# Basic webhook usage
go run main.go -webhook "https://api.example.com/mbs-update"

# With custom headers (e.g., API key authentication)
go run main.go -webhook "https://api.example.com/mbs-update" \
  -webhook-headers '{"Authorization": "Bearer your-token", "X-API-Key": "your-api-key"}'

# With custom headers and force download
go run main.go -force -webhook "https://api.example.com/mbs-update" \
  -webhook-headers '{"Authorization": "Bearer your-token"}'
```

Common header use cases:
- API Key authentication: `{"X-API-Key": "your-api-key"}`
- Bearer token: `{"Authorization": "Bearer your-token"}`
- Custom tracking: `{"X-Request-ID": "unique-id"}`
- Client identification: `{"User-Agent": "MBS-Fetcher/1.0"}`

Note: When providing the headers JSON string on Windows PowerShell or Command Prompt, you may need to escape the quotes differently:
```powershell
# PowerShell
go run main.go -webhook "https://api.example.com/mbs-update" -webhook-headers '{\"Authorization\": \"Bearer your-token\"}'

# Command Prompt
go run main.go -webhook "https://api.example.com/mbs-update" -webhook-headers "{\"Authorization\": \"Bearer your-token\"}"
```

### Force Download (-force)

The -force flag allows you to download and process the MBS data even if the file already exists in the downloads directory.

Use cases:
- Re-download corrupted files
- Update local copy with server changes
- Force type conversion updates
- Test webhook or command execution

Example:
```bash
# Force download even if file exists
go run main.go -force

# Force download and send to webhook
go run main.go -force -webhook "https://api.example.com/mbs-update"
```

## Error Handling

The program includes comprehensive error handling and will display clear error messages if:
- The website is unreachable
- The XML file cannot be downloaded
- The XML to JSON conversion fails
- There are file system permission issues
- The JSON structure is unexpected
- The MBS version date cannot be extracted from the filename
- The command fails to start (for -exec)
- Background command execution fails (logged separately)
- The webhook request fails 