TODO: Compose a list of known issues with the examples, and link to this file from the README.

based on 

		echo "terraform run failed." >&2
		echo "If you see state-lock errors, a previous terraform runner may still be running." >&2
		echo "Fix: stop any stuck container and delete examples/basic/.terraform.tfstate.lock.info (or run tests/terraform-force-unlock-basic.sh)." >&2


echo "If you see AADSTS/tenant-not-found errors, Terraform is reaching real Azure instead of groundstack." >&2
		echo "The optional tls/dns interception stack isn't active yet (examples/edge/certs is currently empty)." >&2
		