let selection: any = []
let sheet: any = {}
let files: any = []
let n = 4
$files | sort-by size -r | into string --decimals 2 | append $n