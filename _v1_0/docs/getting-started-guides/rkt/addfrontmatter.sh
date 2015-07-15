#!/bin/bash

for D in *; do
echo Found dir "${D}"
if [ -d "${D}" ];
then
    for f in *; do
        if [[ $f == *.md ]]
        then
            echo Found file "$f"
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
            echo starting test
            sed -i '' 's/(\(.*\)\.md/(\1.html/g' $f
            echo all href with .md are now .html
        else
            echo "$f" "not a .md file"
        fi
    done
fi
done
echo all directories files processed