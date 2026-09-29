# 012 in nushell pipelines: `sheet` opens the table piped to it in 012
# and returns what you send back, with its types.
#
# Install: 012 nu --install-module, then add `use scripts/012.nu *` to
# config.nu. See https://github.com/FelineStateMachine/012 (docs/nushell).

# Open a table in 012, and return the table sent back on quitting.
#
# The table piped in opens as a sheet, or the file named instead. Edit
# it, sort it or select a range; quitting sends the selection or the
# whole sheet back as NUON, so file sizes, durations and dates come
# back as themselves. Quitting without sending raises an error, so
# `try` can catch it. Text piped in (CSV, TSV, JSON, NUON) is read as
# 012 reads it.
@example "Pick files in 012 and keep filtering" { ls | sheet | where size > 1kb }
@example "Edit a workbook and save what you send back as CSV" { sheet budget.xlsx | save -f budget.csv }
@example "Carry on when nothing is sent back" { try { ps | sheet } catch { [] } }
@search-terms [spreadsheet table edit 012]
export def sheet [
    file?: path # open this workbook or file instead of the input
] {
    let input = $in
    let args = if $file == null { [--pipe --to nuon] } else { [--pipe --to nuon $file] }
    let r = if $input == null {
        ^012 ...$args | complete
    } else if ($input | describe) in [string binary] {
        $input | ^012 ...$args | complete
    } else {
        $input | to nuon | ^012 ...$args | complete
    }
    if $r.exit_code != 0 {
        let why = $r.stderr | str trim | str replace --regex '^012: ' ''
        error make {
            msg: (if $why == "" { $"012 exited with status ($r.exit_code)" } else { $why })
            help: "nothing was sent back; quit with Enter or S to send a table"
        }
    }
    $r.stdout | from nuon
}

# Look at a table in 012, then carry on with nothing.
#
# Like `sheet`, without sending anything back: the table stays in 012
# until you save or download it.
@example "Look at the processes" { ps | sheet view }
@search-terms [spreadsheet table view 012]
export def "sheet view" [] {
    let input = $in
    if ($input | describe) in [string binary] {
        $input | ^012 -
    } else {
        $input | to nuon | ^012 -
    }
    null
}

# Open a nushell notebook in 012.
@example "Open a notebook" { sheet nu work.012 }
export def "sheet nu" [
    file?: path # the workbook to open
] {
    if $file == null { ^012 nu } else { ^012 nu $file }
}
