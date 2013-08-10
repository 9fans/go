package draw

import (
	"fmt"
	"os"
)

func (d *Display) ReadSnarf() (string, error) {
	str, err := d.conn.ReadSnarf()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ReadSnarf: %v\n", err)
		return "", err
	}
	return str, nil
}

func (d *Display) WriteSnarf(str string) error {
	err := d.conn.WriteSnarf(str)
	if err != nil {
		fmt.Fprintf(os.Stderr, "WriteSnarf: %v\n", err)
		return err
	}
	return nil
}
