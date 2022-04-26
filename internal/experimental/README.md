# Experimental APIs

wazero cares about compatability, and also recognizes that practice makes perfect. This area allows us to practice APIs
without leaking them to exported non-internal interfaces. Notably, we sneak configuration through Go context values.
