package abstraction

import (
	"fmt"
	"sync"

	"github.com/sirupsen/logrus"
)

// DriverConstructor is a function that creates a new driver instance
type DriverConstructor func(config interface{}, logger logrus.FieldLogger) (Driver, error)

// driverRegistry holds registered driver constructors
var driverRegistry = struct {
	sync.RWMutex
	constructors map[Type]DriverConstructor
}{
	constructors: make(map[Type]DriverConstructor),
}

// RegisterDriver registers a driver constructor for a given type
func RegisterDriver(driverType Type, constructor DriverConstructor) {
	driverRegistry.Lock()
	defer driverRegistry.Unlock()
	driverRegistry.constructors[driverType] = constructor
}

// GetDriverConstructor returns the constructor for a given driver type
func GetDriverConstructor(driverType Type) (DriverConstructor, error) {
	driverRegistry.RLock()
	defer driverRegistry.RUnlock()

	constructor, ok := driverRegistry.constructors[driverType]
	if !ok {
		return nil, fmt.Errorf("unknown driver type: %s", driverType)
	}

	return constructor, nil
}

// CreateDriver creates a driver using the registered constructor
func CreateDriver(driverType Type, config interface{}, logger logrus.FieldLogger) (Driver, error) {
	constructor, err := GetDriverConstructor(driverType)
	if err != nil {
		return nil, err
	}

	return constructor(config, logger)
}
