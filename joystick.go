package uinput

import (
	"fmt"
	"io"
	"os"
	"syscall"
)

// A Joystick is an input device that uses absolute axis events and button events to simulate a joystick.
type Joystick interface {
	SetAxis(axis uint16, x int32) error
	SetButton(button uint16, on bool) error
	io.Closer
}

type vJoystick struct {
	name       []byte
	deviceFile *os.File
}

// CreateJoystick will create a new joystick device
func CreateJoystick(path string, name []byte, axes []Axis, buttons []Button) (Joystick, error) {
	err := validateDevicePath(path)
	if err != nil {
		return nil, err
	}
	err = validateUinputName(name)
	if err != nil {
		return nil, err
	}

	fd, err := createJoystick(path, name, axes, buttons)
	if err != nil {
		return nil, err
	}

	return vJoystick{name: name, deviceFile: fd}, nil
}

// SetAxis sets the absolute value of an axis
func (vj vJoystick) SetAxis(axis uint16, x int32) error {
	return sendAxisEvent(vj.deviceFile, axis, x)
}

// SetButton sets the state (on or off) of a button
func (vj vJoystick) SetButton(button uint16, on bool) error {
	var state int32
	if on {
		state = 1
	}

	buf, err := inputEventToBuffer(inputEvent{
		Time:  syscall.Timeval{Sec: 0, Usec: 0},
		Type:  evKey,
		Code:  button,
		Value: state,
	})
	if err != nil {
		return fmt.Errorf("key event could not be set: %v", err)
	}

	_, err = vj.deviceFile.Write(buf)
	if err != nil {
		return fmt.Errorf("writing btnEvent structure to the device file failed: %v", err)
	}

	return syncEvents(vj.deviceFile)
}

// Close closes the device and frees up associated resources
func (vj vJoystick) Close() error {
	return closeDevice(vj.deviceFile)
}

// Axis represents an axis
type Axis struct {
	ID  uint16
	Min int32
	Max int32
}

// Button represents a button, hat direction or switch
type Button struct {
	ID uint16
}

func createJoystick(path string, name []byte, axes []Axis, buttons []Button) (fd *os.File, err error) {
	deviceFile, err := createDeviceFile(path)
	if err != nil {
		return nil, fmt.Errorf("could not create absolute axis input device: %v", err)
	}

	err = registerDevice(deviceFile, uintptr(evKey))
	if err != nil {
		deviceFile.Close()
		return nil, fmt.Errorf("failed to register key device: %v", err)
	}

	// register button events
	for _, btn := range buttons {
		err = ioctl(deviceFile, uiSetKeyBit, uintptr(btn.ID))
		if err != nil {
			deviceFile.Close()
			return nil, fmt.Errorf("failed to register button event %v: %v", btn.ID, err)
		}
	}

	err = registerDevice(deviceFile, uintptr(evAbs))
	if err != nil {
		deviceFile.Close()
		return nil, fmt.Errorf("failed to register absolute axis input device: %v", err)
	}

	// register axis events
	var absMin [absSize]int32
	var absMax [absSize]int32
	for _, axis := range axes {
		err = ioctl(deviceFile, uiSetAbsBit, uintptr(axis.ID))
		if err != nil {
			deviceFile.Close()
			return nil, fmt.Errorf("failed to register absolute axis event %v: %v", axis.ID, err)
		}
		absMin[axis.ID] = axis.Min
		absMax[axis.ID] = axis.Max
	}

	return createUsbDevice(deviceFile,
		uinputUserDev{
			Name: toUinputName(name),
			ID: inputID{
				Bustype: 0x06,
				Vendor:  0x01,
				Product: 0x02,
				Version: 0x03,
			},
			Absmin: absMin,
			Absmax: absMax,
		},
	)
}

func sendAxisEvent(deviceFile *os.File, axis uint16, pos int32) error {
	var e inputEvent
	e.Type = evAbs
	e.Code = axis
	e.Value = pos

	buf, err := inputEventToBuffer(e)
	if err != nil {
		return fmt.Errorf("writing abs event failed: %v", err)
	}

	_, err = deviceFile.Write(buf)
	if err != nil {
		return fmt.Errorf("failed to write abs event to device file: %v", err)
	}

	return syncEvents(deviceFile)
}
