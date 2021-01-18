package main

type Err int

const (
	Eopen Err = iota
	Ecreate
	Emenu
	Emodified
	Eio
	Ewseq
	Eunk
	Emissop
	Edelim
	Efork
	Eintr
	Eaddress
	Esearch
	Epattern
	Enewline
	Eblank
	Enopattern
	EnestXY
	Enolbrace
	Enoaddr
	Eoverlap
	Enosub
	Elongrhs
	Ebadrhs
	Erange
	Esequence
	Eorder
	Enoname
	Eleftpar
	Erightpar
	Ebadclass
	Ebadregexp
	Eoverflow
	Enocmd
	Epipe
	Enofile
	Etoolong
	Echanges
	Eempty
	Efsearch
	Emanyfiles
	Elongtag
	Esubexp
	Etmpovfl
	Eappend
	Ecantplumb
	Ebufload
)

type Warn int

const (
	Wdupname Warn = iota
	Wfile
	Wdate
	Wdupfile
	Wnulls
	Wpwd
	Wnotnewline
	Wbadstatus
)
