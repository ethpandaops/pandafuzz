package main
import (
    "encoding/json"
    "fmt"
    "time"
)
type Config struct {
    Duration time.Duration `json:"duration"`
}
func main() {
    // Test unmarshaling from number
    jsonStr := `{"duration": 10000000000}`
    var c Config
    err := json.Unmarshal([]byte(jsonStr), &c)
    fmt.Printf("Unmarshal from number: err=%v, duration=%v\n", err, c.Duration)
    
    // Test marshaling
    c2 := Config{Duration: 10 * time.Second}
    data, _ := json.Marshal(c2)
    fmt.Printf("Marshal result: %s\n", data)
}
