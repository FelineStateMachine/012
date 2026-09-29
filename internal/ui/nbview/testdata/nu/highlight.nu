let selection: any = []
let sheet: any = {}
let files: any = []
      $files | where size > 1kb | sort-by size --reverse # largest