package gmaps

func completeAddressFromPlaceArray(business []any) Address {
	const addressNode = 183

	return Address{
		Borough:    getNthElementAndCast[string](business, addressNode, 1, 0),
		Street:     getNthElementAndCast[string](business, addressNode, 1, 1),
		City:       getNthElementAndCast[string](business, addressNode, 1, 3),
		PostalCode: getNthElementAndCast[string](business, addressNode, 1, 4),
		State:      getNthElementAndCast[string](business, addressNode, 1, 5),
		Country:    getNthElementAndCast[string](business, addressNode, 1, 6),
	}
}
