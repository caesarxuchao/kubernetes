#!/bin/bash
	for f in *; do
		if [[ $f == *.md ]]
		then
			echo Found "$f";
			if grep -e '---' "$f"
			then
				echo Already has front matter: "$f"
			else
				sed -i '' '1i\
					---\
					layout: docwithnav\
					---\
					' $f
				echo SUCCESSFULLY UPDATED: "$f"
			fi
		else
			echo "$f" "not a .md file"
		fi
	done