# The vault

The vault keeps memories and conversations out of the rest of Afterfeed. Use it for things you want to keep but not see by accident, for example when you show your feed to someone.

## Using the vault

- **Move something in:** open the **⋯** menu on a memory or a conversation and choose **Move to vault**.
- **Open the vault:** choose **Vault** in the menu and enter your six-digit PIN. The first time, you set the PIN.
- **Move something out:** open it in the vault and choose **Remove from vault** in the **⋯** menu.

What is in the vault does not appear in Today, Years, the map, Messages, search results or any counts.

The vault locks again after a few minutes without activity, when you sign out, and when you choose **Lock**. The time is set under **Settings › Vault**.

## Changing the PIN

Go to **Settings › Vault** and choose **Change PIN**. You need the current PIN.

## Forgotten PIN

Choose **Forgot PIN?** and enter your account password. The PIN is removed and you can set a new one. The contents stay in the vault.

You can also remove the PIN from the server:

```sh
docker exec -it afterfeed afterfeed auth reset-vault-pin
```

## What the vault protects

The PIN protects the interface. It does not encrypt the files: anyone with access to your server or your data volume can still open them. See [Security](/security).
