# GDL (Go Downloader)

A high-performance CLI downloader with support for segmented downloads, batch processing, and cloud storage links (Google Drive, OneDrive).

## 🚀 Installation & Build

```bash
# Build the binary
go build -o gdl main.go
```

## 📖 Usage Examples

### 1. Simple Download
Download a file to the current directory.
```bash
./gdl download https://example.com/large_file.zip
```

### 2. Custom Output
Specify filename (`-o`) and directory (`-d`).
```bash
./gdl download -o my_app.zip -d ./downloads https://example.com/app_v1.zip
```

### 3. High Concurrency
Increase the number of connections (`-c`) for faster speeds (default is 8).
```bash
./gdl download -c 16 https://example.com/huge_dataset.csv
```

### 4. Google Drive & OneDrive
Directly download from share links (auto-handles virus warnings and direct link conversion).

**Google Drive:**
```bash
./gdl download https://drive.google.com/file/d/1A2B3C4D5E6F7G8H9I0J/view
```

**OneDrive:**
```bash
./gdl download https://1drv.ms/u/s!Am...
```

### 5. Batch Download
Download multiple files from a text file (one URL per line).

**Create `urls.txt`:**
```text
https://example.com/file1.zip
https://drive.google.com/file/d/123.../view
https://1drv.ms/u/s!abc...
```

**Run Batch:**
```bash
./gdl batch urls.txt -d ./batch_output -c 8
```
