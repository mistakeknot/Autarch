# Where scripts come from and how to run one

When a machine change needs your hands, a thread hands you one script as `bash /absolute/path`, with its sha256.

- Check the sha256 against the card before you run it.
- Run it with `--check` first: it changes nothing and prints what it would do.
- The script reports its result back to the thread that wrote it. You do not need to paste the output.
- Run it as yourself in a root shell; it switches to your user where it must.
