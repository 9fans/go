package memdraw

import "9fans.net/go/draw"

type Refreshfn func(*Image, draw.Rectangle, interface{})

type Screen struct {
	Frontmost *Image /* frontmost layer on screen */
	Rearmost  *Image /* rearmost layer on screen */
	Image     *Image /* upon which all layers are drawn */
	Fill      *Image /* if non-zero, picture to use when repainting */
}

type Layer struct {
	Screenr    draw.Rectangle /* true position of layer on screen */
	Delta      draw.Point     /* add delta to go from image coords to screen */
	Screen     *Screen        /* screen this layer belongs to */
	front      *Image         /* window in front of this one */
	rear       *Image         /* window behind this one*/
	clear      bool           /* layer is fully visible */
	save       *Image         /* save area for obscured parts */
	Refreshfn  Refreshfn      /* function to call to refresh obscured parts if save==nil */
	Refreshptr interface{}    /* argument to refreshfn */
}
