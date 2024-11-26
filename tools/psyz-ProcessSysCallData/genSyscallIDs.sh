printf '#include <sys/syscall.h>\n' | gcc -dD -E - | awk '$1 == "#define" { m[$2] = $3 } END { for (name in m) if (name ~ /^SYS_/) { v = name; while (v in m) v = m[v]; sub(/^SYS_/, "", name); printf "%s %s\n", v, name } }'  > syscallIDs.txt
:
