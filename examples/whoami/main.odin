// Minimal live check of the generated SDK: prints the authorised user.
//
//	POCKETSMITH_DEVELOPER_KEY=... odin run examples/whoami
package whoami

import "core:fmt"
import "core:os"
import ps "../../sdk/pocketsmith"

main :: proc() {
	key := os.get_env("POCKETSMITH_DEVELOPER_KEY", context.allocator)
	defer delete(key)
	if key == "" {
		fmt.eprintln("set POCKETSMITH_DEVELOPER_KEY to a personal developer key (https://my.pocketsmith.com/security)")
		os.exit(1)
	}

	client := ps.client_from_developer_key(key)
	user, err := ps.get_authorised_user(&client)
	if err != nil {
		fmt.eprintln("error:", err)
		os.exit(1)
	}
	fmt.printfln("user #%v: %s <%s>, base currency %s, %v transaction accounts available",
		user.id, user.login, user.email, user.base_currency_code, user.available_accounts)
}
