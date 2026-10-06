# Fail when internal/flexera or internal/flexeraapi statement coverage is under min.
BEGIN {
	if (min == "") {
		min = 80
	}
	wanted["internal/flexera"] = 1
	wanted["internal/flexeraapi"] = 1
}

/^mode:/ {
	next
}

{
	pkg = ""
	if (index($1, "/internal/flexeraapi/")) {
		pkg = "internal/flexeraapi"
	} else if (index($1, "/internal/flexera/")) {
		pkg = "internal/flexera"
	}
	if (pkg == "") {
		next
	}
	stmts = $(NF - 1) + 0
	hits = $NF + 0
	total[pkg] += stmts
	if (hits > 0) {
		covered[pkg] += stmts
	}
}

END {
	fail = 0
	for (pkg in wanted) {
		if (total[pkg] + 0 == 0) {
			printf "coverage gate: %s missing from profile\n", pkg > "/dev/stderr"
			fail = 1
			continue
		}
		pct10 = int((covered[pkg] * 1000) / total[pkg])
		printf "%s %d.%d%%\n", pkg, int(pct10 / 10), pct10 % 10
		if (covered[pkg] * 1000 < total[pkg] * (min * 10)) {
			printf "coverage gate: %s is below %s%%\n", pkg, min > "/dev/stderr"
			fail = 1
		}
	}
	exit fail
}
