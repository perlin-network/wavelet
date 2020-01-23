## Usage

1. Generate private keys (set `COUNT` to number of nodes):

   ```sh
	 $ COUNT=5 make private_keys
	 ```

2. Deploy:

   ```sh
	 $ export TF_VAR_key_name="mykeyname"
	 $ make terraform
	 ```

3. Don't forget to destroy once not needed:

	 ```sh
	 $ make clean
	 ```

