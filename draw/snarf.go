package draw

import (
	"fmt"
	"os"
)

func (d *Display) ReadSnarf() ([]byte, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	data, err := d.conn.ReadSnarf()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ReadSnarf: %v\n", err)
		return nil, err
	}
	return data, nil
}

func (d *Display) WriteSnarf(data []byte) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	err := d.conn.WriteSnarf(data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "WriteSnarf: %v\n", err)
		return err
	}
	return nil
}
