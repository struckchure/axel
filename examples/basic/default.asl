# Axel Schema Language example
# File extension: .asl

abstract type Base {
  required id: uuid {
    default := gen_uuid();
    constraint exclusive;
    constraint pk;
  };
  required created_at: datetime { default := datetime_current(); };
  required updated_at: datetime { default := datetime_current(); };
}

type User extending Base {
  required email: str {
    constraint exclusive;
    constraint min_length(10);
    constraint max_length(100);
  };
  name: str {
    default := 'n/a';
  };
  required age: int32;
  required health: int32;
  active: bool { default := true };
}

type Comment extending Base {
  required link post: Post;
  required content: str;
  required link author: User;
}

type Post extending Base {
  required title: str;
  required content: str;
  required link author: User;
  multi link likes: User;
}
