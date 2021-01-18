#!/usr/local/plan9/bin/rc

sam=./sam
if(~ $1 -std) {
	sam=$PLAN9/bin/sam
	shift
}
if(~ $#* 0)
	*=(testdata/*.txt)
if(~ $sam(1) ./sam)
	go build || exit 1
fail=()
for(i) {
	rm -f tmp tmp2
	echo '#' $i
	sed -n '/^-- out --$/,$p' $i | sed 1d >test.want
	sed '/^-- out --$/q' $i | sed '/^-- out --$/d' |
		$sam -d >[2=1] | sed '
			s/ *$//
			s/No such file/no such file/
		' > test.have
	if (! 9 diff -c test.want test.have >test.diff) {
		echo FAIL with diff:
		cat test.diff
		fail=($fail $i)
		exit 1
	}
}
if(! ~ $#fail 0) {
	echo FAILED: $fail
	exit 1
}
rm -f test.have test.want test.err test.diff
exit 0
