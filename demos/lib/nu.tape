# A nushell session for the nushell tapes: nu without your config files,
# a fixed prompt, and 012 on the PATH, so what's typed is what you'd
# type. Source it after setup.tape; it leaves the screen clear, hidden.
Hide
Type `nu --no-config-file -e '$env.PROMPT_COMMAND = {|| "demos" }; $env.PROMPT_COMMAND_RIGHT = ""; $env.PATH = ($env.PATH | prepend ("../bin" | path expand))'` Enter
Sleep 1.5s
Type "clear" Enter
Sleep 500ms
