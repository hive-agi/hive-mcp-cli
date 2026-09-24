# Stage: `hive setup`, exactly as the installer's closing message tells the customer.
export PATH="$HOME/.local/bin:$PATH"
hive setup 2>&1 | grep -vE 'Downloading|Extracting|Pulling|Waiting|Verifying|^Get:|^Unpacking|^Selecting|^Preparing|^Setting up|^Processing'
exit "${PIPESTATUS[0]}"
