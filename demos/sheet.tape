# The sheet command: nushell's module from 012 nu --install-module, used
# in a nu session. ls's table goes through sheet, three rows are sent
# back and nu keeps filtering them by size; then quitting without
# sending makes sheet fail with 012's reason, which try catches. Needs
# nu on the PATH.
Output out/sheet.gif
Source lib/setup.tape
Set Theme "tokyonight"
Source lib/nu.tape

# The module goes next to the binary, out of the recording.
Hide
Type "^012 nu --install-module ../bin/012.nu" Enter
Sleep 1s
Type "use ../bin/012.nu *; clear" Enter
Sleep 1s
Show

Sleep 600ms
Type "ls data | select name size | sort-by size --reverse | sheet | where size > 1kb"
Sleep 600ms
Enter
Sleep 2s
Ctrl+G
Type "A1:B4" Enter
Sleep 1s
Screenshot out/stills/sheet-sheet.png
Ctrl+Q
Sleep 1.6s
Enter
Sleep 2.5s
Screenshot out/stills/sheet-result.png
Type "try { ls data | sheet } catch {|e| $e.msg }"
Sleep 600ms
Enter
Sleep 2s
Ctrl+Q
Sleep 1.4s
Type "d"
Sleep 2.5s
Screenshot out/stills/sheet-error.png
Sleep 1s
