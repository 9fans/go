package draw

import (
	"fmt"
	"os"
)

func (d *Display) ReadSnarf(b []byte) (int, int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	n, actual, err := d.conn.ReadSnarf(b)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ReadSnarf: %v\n", err)
		return 0, 0, err
	}
	return n, actual, nil
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
