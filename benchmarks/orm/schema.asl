type Account {
  required id:      int32  { constraint pk; };
  required email:   str;
  required age:     int32;
  required active:  bool;
}

type Post {
  required id:           int32    { constraint pk; };
  required title:        str;
  required link author:  Account;
}
