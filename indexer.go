package main

import (
	"app/indexer/types"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	_ "net/http/pprof"
	"net/mail"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	wg          sync.WaitGroup
	writeWg     sync.WaitGroup
	files       = make(chan string)
	rejectFiles = make(chan types.Email, 10)
	sem         = make(chan struct{}, 3) // Crear un canal semáforo con capacidad 7
	numCores    = runtime.NumCPU()
	dir         = os.Getenv("ZINCSEARCH_FILES_DIR")
	indexName   = os.Getenv("ZINCSEARCH_INDEX")
	user        = os.Getenv("ZINC_FIRST_ADMIN_USER")
	password    = os.Getenv("ZINC_FIRST_ADMIN_PASSWORD")
	host        = fmt.Sprintf(`http://%s:%s`, os.Getenv("ZINCSEARCH_IP"), os.Getenv("ZINCSEARCH_PORT"))
	client      = &http.Client{}
)

func main() {
	mf, err := os.Create("mem_profile.pprof")
	if err != nil {
		fmt.Println("Error creando el perfil de memoria:", err)
		return
	}
	defer mf.Close()

	f, err := os.Create("cpu_profile.pprof")
	if err != nil {
		fmt.Println("Error creando el perfil de CPU:", err)
		return
	}
	defer f.Close()

	if err := pprof.StartCPUProfile(f); err != nil {
		fmt.Println("Error iniciando el perfil de CPU:", err)
		return
	}
	defer pprof.StopCPUProfile()

	deleteIndex()
	createIndex()

	ti := time.Now()

	wg.Add(1)
	go func() {
		readDir(dir)
	}()

	go func() {
		wg.Wait()
		close(files)
	}()

	// Almacenar todos los elementos del canal en un arreglo
	var allFiles []string
	for file := range files {
		allFiles = append(allFiles, file)
	}

	batchSize := 2500 * numCores
	countFiles := len(allFiles)
	if countFiles < batchSize {
		batchSize = countFiles
	}
	routines := int(math.Ceil(float64(countFiles) / float64(batchSize)))

	fmt.Println("Número de cores: ", numCores)
	fmt.Println("Archivos totales: ", countFiles)
	fmt.Println("Número de procesos: ", routines)
	fmt.Println("Archivos para batch: ", batchSize)

	for i := 0; i < routines; i++ {
		start := i * batchSize
		end := (i + 1) * batchSize
		if end > countFiles {
			end = countFiles
		}
		writeWg.Add(1)
		sem <- struct{}{} // Bloquear el canal semáforo antes de ejecutar la función
		go processBatch(start, end, allFiles)
	}

	go func() {
		writeWg.Wait()
		close(rejectFiles)
	}()

	for file := range rejectFiles {
		fmt.Println(file.From)
		processRejectedFiles(file)
	}

	fmt.Println("Duracion: ", time.Since(ti))

	if err := pprof.WriteHeapProfile(mf); err != nil {
		fmt.Println("Error escribiendo el perfil de memoria:", err)
		return
	}
}

func processRejectedFiles(file types.Email) {
	var parts []string
	// Decodificar el string ASCII codificado
	res, err := strconv.Unquote(file.Content)
	if err != nil {
		return
	}

	file.Content = res
	pos := len(file.Content) / 2
	parts = append(parts, file.Content[:pos], file.Content[pos:])
	for _, part := range parts {
		file.Content = strconv.QuoteToASCII(part)
		jsonData, err := json.Marshal(file)
		if err != nil {
			continue
		}
		if len(jsonData) > 976000 {
			processRejectedFiles(file)
		} else {
			httpExec("POST", "/api/"+indexName+"/_multi", string(jsonData))
		}
	}
}

func processBatch(start int, end int, allFiles []string) {
	defer func() {
		<-sem // Liberar el semáforo después de que la rutina ha terminado
		writeWg.Done()
	}()
	var result bytes.Buffer
	for i := start; i < end; i++ {
		data, err := contentParseToJson(allFiles[i])
		if err == nil {
			result.Write(data)
			result.WriteString("\n")
		}
	}
	httpExec("POST", "/api/"+indexName+"/_multi", result.String())
}

func contentParseToJson(path string) ([]byte, error) {
	f, err := os.Open(path)

	if err != nil {
		return nil, err
	}

	defer f.Close() // Deferimos el cierre del archivo hasta que la función termine

	m, err := mail.ReadMessage(f)

	if err != nil {
		//fmt.Println("Refused file: ", path)
		return nil, err
	}

	header := m.Header
	content, err := io.ReadAll(m.Body)

	if err != nil {
		return nil, err
	}

	data := types.Email{
		From:      header.Get("From"),
		To:        header.Get("To"),
		Subject:   header.Get("Subject"),
		Date:      header.Get("Date"),
		MessageId: header.Get("Message-ID"),
		Content:   strconv.QuoteToASCII(string(content)),
	}
	jsonData, err := json.Marshal(data)

	if err != nil {
		return nil, err
	}

	if len(jsonData) > 976000 {
		rejectFiles <- data
		return nil, fmt.Errorf("error")
	}

	return jsonData, nil
}

func readDir(dir string) {
	defer wg.Done()

	content, err := os.ReadDir(dir)
	if err == nil {
		for _, element := range content {
			if element.IsDir() {
				wg.Add(1)
				go readDir(filepath.Join(dir, element.Name()))
			} else {
				files <- filepath.Join(dir, element.Name())
			}
		}
	}
}
func httpExec(verbParam string, path string, body string) {
	req, err := http.NewRequest(verbParam, host+path, strings.NewReader(body))
	if err == nil {
		// Set headers if needed
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(user+":"+password)))

		// Make the PUT request
		response, err := client.Do(req)
		if err == nil {
			defer response.Body.Close()
		}
	}
}

func createIndex() {
	body := fmt.Sprintf(`{
        "name": "%s",
        "storage_type": "disk",
        "shard_num": 1,
        "mappings": {
            "properties": {
                "to": {
                    "type": "text",
                    "index": true,
                    "store": true,
                    "highlightable": true
                },"from": {
                    "type": "text",
                    "index": true,
                    "store": true,
                    "highlightable": true
                },"date": {
                    "type": "text",
                    "index": true,
                    "store": true,
                    "highlightable": true
                },"subject": {
                    "type": "text",
                    "index": true,
                    "store": true,
                    "highlightable": true
                },"body": {
                    "type": "text",
                    "index": true,
                    "store": true,
                    "highlightable": true
                },"messageId": {
                    "type": "text",
                    "index": true,
                    "store": true,
                    "highlightable": true
                }
            }
        }
    }`, indexName)
	httpExec("POST", "/api/index", body)
}

func deleteIndex() {
	httpExec("DELETE", fmt.Sprintf(`/api/index/%s`, indexName), "")
}
